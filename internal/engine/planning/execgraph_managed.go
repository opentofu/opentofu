// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"fmt"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
)

////////////////////////////////////////////////////////////////////////////////
// This file contains methods of [execGraphBuilder] that are related to the
// parts of an execution graph that deal with resource instances of mode
// [addrs.ManagedResourceMode] in particular.
////////////////////////////////////////////////////////////////////////////////

// ManagedResourceSubgraph adds graph nodes needed to apply changes for a
// managed resource instance, and returns various items needed to describe
// its relationships with other resource instance and provider instance
// subgraphs.
func (b *execGraphBuilder) ManagedResourceInstanceSubgraph(
	plannedChange *plans.ResourceInstanceChange,
	effectiveReplaceOrder resourceInstanceReplaceOrder,
) resourceInstanceObjectSubgraph {
	// Before we go any further we'll just make sure what we've been given
	// is sensible, so that the remaining code can assume the following
	// about the given change. Any panics in the following suggest that there's
	// a bug in the caller, unless we're intentionally changing the rules
	// for what the different action types represent.
	if plannedChange.DeposedKey != states.NotDeposed && plannedChange.Action != plans.Delete {
		// The only sensible thing to do with a deposed object is to delete it.
		panic(fmt.Sprintf("invalid action %s for %s deposed object %s", plannedChange.Action, plannedChange.PrevRunAddr, plannedChange.DeposedKey))
	}
	if plannedChange.Action == plans.Create && !plannedChange.Before.IsNull() {
		panic(fmt.Sprintf("for %s has action %s but non-null prior value", plannedChange.Addr, plannedChange.Action))
	}
	if (plannedChange.Action == plans.Delete || plannedChange.Action == plans.Forget) && !plannedChange.After.IsNull() {
		panic(fmt.Sprintf("change for %s has action %s but non-null planned new value", plannedChange.PrevRunAddr, plannedChange.Action))
	}
	if plannedChange.Action != plans.Create && plannedChange.Action != plans.Delete && plannedChange.Action != plans.Forget && (plannedChange.Before.IsNull() || plannedChange.After.IsNull()) {
		panic(fmt.Sprintf("change for %s has action %s but does not have both a before and after value", plannedChange.PrevRunAddr, plannedChange.Action))
	}

	changeAction := plannedChange.Action
	if changeAction.IsReplace() {
		// The effective replace order finalizes which of the two replace
		// actions we will actually use.
		changeAction = effectiveReplaceOrder.ChangeAction()
	}

	// The shape of execution subgraph we generate here varies depending on
	// which change action was planned.
	switch changeAction {
	case plans.Create, plans.Update:
		return b.managedResourceInstanceSubgraphCreateOrUpdate(plannedChange)
	case plans.Delete:
		return b.managedResourceInstanceSubgraphDelete(plannedChange)
	case plans.Forget:
		return b.managedResourceInstanceSubgraphForget(plannedChange)
	case plans.DeleteThenCreate, plans.ForgetThenCreate:
		return b.managedResourceInstanceSubgraphDeleteOrForgetThenCreate(plannedChange)
	case plans.CreateThenDelete:
		return b.managedResourceInstanceSubgraphCreateThenDelete(plannedChange)
	case plans.NoOp:
		// TODO: We need to handle this because it can occur if the
		// configuration hasn't changed but the object will move to a new
		// resource instance address during the apply phase.
		panic("plans.NoOp execution graph not yet implemented")
	default:
		// We should not get here: the cases above should cover every action
		// that [planGlue.planDesiredManagedResourceInstance] can possibly
		// produce.
		panic(fmt.Sprintf("unsupported change action %s for %s", plannedChange.Action, plannedChange.Addr))
	}
}

func (b *execGraphBuilder) managedResourceInstanceSubgraphCreateOrUpdate(
	plannedChange *plans.ResourceInstanceChange,
) resourceInstanceObjectSubgraph {
	metadataRef, desiredRef, priorRef, plannedValueRef := b.managedResourceInstanceChangeInputs(plannedChange)
	finalPlanRef := b.lower.ManagedFinalPlan(
		metadataRef,
		desiredRef,
		priorRef,
		plannedValueRef,
	)
	waitFor, addDesiredDep := b.lower.MutableWaiter()
	valueRef := b.lower.ManagedApply(
		finalPlanRef,
		execgraph.NilResultRef[*exec.ResourceInstanceObject](),
		waitFor,
	)
	return resourceInstanceObjectSubgraph{
		valueRef:      valueRef,
		addDesiredDep: addDesiredDep,
	}
}

func (b *execGraphBuilder) managedResourceInstanceSubgraphDelete(
	plannedChange *plans.ResourceInstanceChange,
) resourceInstanceObjectSubgraph {
	metadataRef, desiredRef, priorRef, plannedValueRef := b.managedResourceInstanceChangeInputs(plannedChange)
	waitFor, addDeleteDep := b.lower.MutableWaiter()
	finalPlanRef := b.lower.ManagedFinalPlan(
		metadataRef,
		desiredRef, // always produces nil in this case
		priorRef,
		plannedValueRef,
	)
	deletionRef := b.lower.ManagedApply(
		finalPlanRef,
		execgraph.NilResultRef[*exec.ResourceInstanceObject](),
		waitFor,
	)
	return resourceInstanceObjectSubgraph{
		// We report the prior state reference as the "valueRef" for a delete.
		// In a normal plan that doesn't do anything because nothing is allowed
		// to refer to a resource instance that's being deleted anyway, but
		// it's important for destroy-mode plans because annoyingly it _is_ valid
		// to refer to an object being deleted in that case, so that ephemeral
		// objects like provider instances can configure themselves based on
		// the prior state before the object is deleted.
		valueRef:     priorRef,
		deletionRef:  deletionRef,
		addOrphanDep: addDeleteDep,
	}
}

func (b *execGraphBuilder) managedResourceInstanceSubgraphForget(
	plannedChange *plans.ResourceInstanceChange,
) resourceInstanceObjectSubgraph {
	// TODO: Add a new execgraph opcode ManagedForget and use that here.
	panic("execgraph for Forget not yet implemented")
}

func (b *execGraphBuilder) managedResourceInstanceSubgraphDeleteOrForgetThenCreate(
	plannedChange *plans.ResourceInstanceChange,
) resourceInstanceObjectSubgraph {
	if plannedChange.Action == plans.ForgetThenCreate {
		// TODO: Implement this action too, which is similar but with the
		// "delete" let replaced with something like what
		// managedResourceInstanceSubgraphForget would generate.
		panic("execgraph for ForgetThenCreate not yet implemented")
	}

	desiredWaitFor, addCreateDep := b.lower.MutableWaiter()
	deleteWaitFor, addDeleteDep := b.lower.MutableWaiter()

	// This has much the same _effect_ as the separate delete and create
	// actions chained together, but we arrange the operations in such a
	// way that the delete leg can't start unless the desired state is
	// successfully evaluated.
	metadataRef, desiredRef, priorRef, plannedValueRef := b.managedResourceInstanceChangeInputs(plannedChange)

	// We plan both the create and destroy parts of this process before we
	// make any real changes, to reduce the risk that we'll be left in a
	// partially-applied state where neither object exists. (Though of course
	// that's always possible, if the "create" step fails at apply.)
	createPlanRef := b.lower.ManagedFinalPlan(
		metadataRef,
		desiredRef,
		execgraph.NilResultRef[*exec.ResourceInstanceObject](),
		plannedValueRef,
	)
	destroyPlanRef := b.lower.ManagedFinalPlan(
		metadataRef,
		execgraph.NilResultRef[*eval.DesiredResourceInstance](),
		priorRef,
		b.lower.ConstantValue(cty.NullVal(
			// TODO: is this okay or do we need to use the type constraint derived from the schema?
			// The two could differ for resource types that have cty.DynamicPseudoType
			// attributes, like in kubernetes_manifest from the hashicorp/kubernetes provider,
			// where here we'd capture the type of the current manifest instead of recording
			// that the manifest's type is unknown. However, we don't typically fuss too much
			// about the exact type of a null, so this is probably fine.
			plannedChange.After.Type(),
		)),
	)
	addDeleteDep(createPlanRef) // must successfully plan for creation before applying deletion
	destroyResultRef := b.lower.ManagedApply(
		destroyPlanRef,
		execgraph.NilResultRef[*exec.ResourceInstanceObject](),
		deleteWaitFor,
	)
	addCreateDep(destroyResultRef) // must finish deleting before creating
	createResultRef := b.lower.ManagedApply(
		createPlanRef,
		execgraph.NilResultRef[*exec.ResourceInstanceObject](),
		desiredWaitFor,
	)

	return resourceInstanceObjectSubgraph{
		valueRef:      createResultRef,
		deletionRef:   destroyResultRef,
		addDesiredDep: addCreateDep,
		addOrphanDep:  addDeleteDep,
	}
}

func (b *execGraphBuilder) managedResourceInstanceSubgraphCreateThenDelete(
	plannedChange *plans.ResourceInstanceChange,
) resourceInstanceObjectSubgraph {
	desiredWaitFor, addCreateDep := b.lower.MutableWaiter()
	deleteWaitFor, addDeleteDep := b.lower.MutableWaiter()

	// This has much the same effect as the separate create and delete
	// actions chained together, but we arrange the operations in such a
	// way that we don't make any changes unless we can produce valid final
	// plans for both changes.
	metadataRef, desiredRef, priorRef, plannedValueRef := b.managedResourceInstanceChangeInputs(plannedChange)

	// We plan both the create and destroy parts of this process before we
	// make any real changes, to reduce the risk that we'll be left in a
	// partially-applied state where we're left with a deposed object present
	// in the final state.
	createPlanRef := b.lower.ManagedFinalPlan(
		metadataRef,
		desiredRef,
		execgraph.NilResultRef[*exec.ResourceInstanceObject](),
		plannedValueRef,
	)
	destroyPlanRef := b.lower.ManagedFinalPlan(
		metadataRef,
		execgraph.NilResultRef[*eval.DesiredResourceInstance](),
		priorRef,
		b.lower.ConstantValue(cty.NullVal(
			// TODO: is this okay or do we need to use the type constraint derived from the schema?
			// The two could differ for resource types that have cty.DynamicPseudoType
			// attributes, like in kubernetes_manifest from the hashicorp/kubernetes provider,
			// where here we'd capture the type of the current manifest instead of recording
			// that the manifest's type is unknown. However, we don't typically fuss too much
			// about the exact type of a null, so this is probably fine.
			plannedChange.After.Type(),
		)),
	)
	// We'll now reinterpret the destroy plan as being for the deposed key
	// that we'll retain this object under until the new object has been
	// created, or failed to be created.
	deposedKey := b.lower.ConstantDeposedKey(b.makeDeposedKey(plannedChange.Addr))
	destroyPlanRef = b.lower.ManagedPrepareDepose(destroyPlanRef, deposedKey)

	// Unlike most subgraphs where we put all of the forced dependencies
	// directly on the ManagedApply, for create-before-destroy we place them
	// on ManagedPerformDepose so that we'll delay deposing the previous
	// object until we're definitely ready to attempt creating the new
	// object, so we can avoid being left with a deposed object and no
	// current object. The ManagedApply operation should not have any
	// dependencies that ManagedPerformDepose does not also have.
	addCreateDep(createPlanRef) // don't depose unless we successfully create a plan
	deposedObjRef := b.lower.ManagedPerformDepose(
		priorRef,
		destroyPlanRef,
		desiredWaitFor,
	)
	createResultRef := b.lower.ManagedApply(
		createPlanRef,
		deposedObjRef,    // will be restored as current if creation completely fails
		b.lower.Waiter(), // forced dependencies are on ManagedPerformDeposed instead; see above
		// Note that this "create" operation indirectly depends on planning
		// the destroy action due to the deposedObjRef argument above, so we
		// don't need an additional direct dependency on destroyPlanRef here.
	)
	// No other resource instances can depend on the value from the destroy
	// result, so if the destroy fails after the create succeeded then we can
	// proceed with applying any downstream changes that refer to what we
	// created and then we'll end with the deposed object still in the state and
	// error diagnostics explaining why destroying it didn't work.
	addDeleteDep(createResultRef) // delete must not begin until creation has succeeded
	deletionRef := b.lower.ManagedApply(
		destroyPlanRef,
		execgraph.NilResultRef[*exec.ResourceInstanceObject](),
		deleteWaitFor,
	)

	return resourceInstanceObjectSubgraph{
		valueRef:      createResultRef,
		deletionRef:   deletionRef,
		addDesiredDep: addCreateDep,
		addOrphanDep:  addDeleteDep,
	}
}

// managedResourceInstanceChangeInputs adds to the graph the operations needed
// to obtain the metadata, desired object, and prior object needed to
// perform the given resource instance change during the apply phase.
//
// The "metadata" result always provides a non-nil value if it succeeds. The
// "desired" and "prior" results may each produce a nil value depending on
// the action specified in the resource instance change.
func (b *execGraphBuilder) managedResourceInstanceChangeInputs(
	plannedChange *plans.ResourceInstanceChange,
) (
	metadata execgraph.ResultRef[*exec.ResourceInstanceObjectMeta],
	desired execgraph.ResultRef[*eval.DesiredResourceInstance],
	prior execgraph.ResourceInstanceResultRef,
	plannedValue execgraph.ResultRef[cty.Value],
) {
	unmarkedAfter, _ := plannedChange.After.UnmarkDeep()
	plannedValue = b.lower.ConstantValue(unmarkedAfter)
	if plannedChange.DeposedKey != states.NotDeposed {
		// We use different operations to access deposed objects.
		if plannedChange.Action != plans.Delete {
			// Delete is the only reasonable action for a deposed object.
			panic(fmt.Sprintf("cannot perform %s on %s", plannedChange.Action, plannedChange.PrevRunAddr))
		}
		prevAddrRef := b.lower.ConstantResourceInstAddr(plannedChange.PrevRunAddr)
		dkRef := b.lower.ConstantDeposedKey(plannedChange.DeposedKey)
		prior = b.lower.ManagedAlreadyDeposed(prevAddrRef, dkRef)
		metadata = b.lower.ManagedDeposedMeta(prevAddrRef, dkRef, prior)
		return metadata, execgraph.NilResultRef[*eval.DesiredResourceInstance](), prior, plannedValue
	}

	switch plannedChange.Action {
	case plans.Create:
		newAddrRef := b.lower.ConstantResourceInstAddr(plannedChange.Addr)
		metadata = b.lower.ResourceInstanceCurrentMeta(newAddrRef, execgraph.NilResultRef[*exec.ResourceInstanceObject]())
		desired = b.lower.ResourceInstanceDesired(metadata)
	case plans.Delete:
		prevAddrRef := b.lower.ConstantResourceInstAddr(plannedChange.PrevRunAddr)
		prior = b.lower.ResourceInstancePrior(prevAddrRef)
		metadata = b.lower.ResourceInstanceCurrentMeta(prevAddrRef, prior)
	default:
		var newAddrRef, prevAddrRef execgraph.ResultRef[addrs.AbsResourceInstance]
		newAddrRef = b.lower.ConstantResourceInstAddr(plannedChange.Addr)
		if plannedChange.PrevRunAddr.Equal(plannedChange.Addr) {
			prevAddrRef = newAddrRef
		} else {
			prevAddrRef = b.lower.ConstantResourceInstAddr(plannedChange.PrevRunAddr)
		}

		prior = b.lower.ResourceInstancePrior(prevAddrRef)
		metadata = b.lower.ResourceInstanceCurrentMeta(newAddrRef, prior)
		desired = b.lower.ResourceInstanceDesired(metadata)
		if !plannedChange.PrevRunAddr.Equal(plannedChange.Addr) {
			prior = b.lower.ManagedChangeAddr(prior, newAddrRef)
		}
	}
	return metadata, desired, prior, plannedValue
}
