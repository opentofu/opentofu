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
// managed resource instance, and returns what should be used as its final
// result to propagate into to downstream references.
//
// TODO: This is definitely not sufficient for the full complexity of all of the
// different ways managed resources can potentially need to be handled in an
// execution graph. It's just a simple placeholder adapted from code that was
// originally written inline in [planGlue.planDesiredManagedResourceInstance]
// just to preserve the existing functionality for now until we design a more
// complete approach in later work.
func (b *execGraphBuilder) ManagedResourceInstanceSubgraph(
	plannedChange *plans.ResourceInstanceChange,
	providerClientRef execgraph.ResultRef[*exec.ProviderClient],
	requiredResourceInstances addrs.Set[addrs.AbsResourceInstance],
) execgraph.ResourceInstanceResultRef {
	b.mu.Lock()
	defer b.mu.Unlock()

	// We need to explicitly model our dependency on any upstream resource
	// instances in the resource instance graph. These don't naturally emerge
	// from the data flow because these results are intermediated through the
	// evaluator, which indirectly incorporates the results into the
	// desiredInstRef result we'll build below.
	dependencyWaiter, closeDependencyAfter := b.waiterForResourceInstances(requiredResourceInstances.All())

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

	// The shape of execution subgraph we generate here varies depending on
	// which change action was planned.
	var finalResultRef execgraph.ResourceInstanceResultRef
	switch plannedChange.Action {
	case plans.Create:
		finalResultRef = b.managedResourceInstanceSubgraphCreate(plannedChange, providerClientRef, dependencyWaiter)
	case plans.Delete:
		finalResultRef = b.managedResourceInstanceSubgraphDelete(plannedChange, providerClientRef)
	case plans.Update:
		finalResultRef = b.managedResourceInstanceSubgraphUpdate(plannedChange, providerClientRef, dependencyWaiter)
	case plans.Forget:
		finalResultRef = b.managedResourceInstanceSubgraphForget(plannedChange, providerClientRef)
	case plans.DeleteThenCreate, plans.ForgetThenCreate:
		finalResultRef = b.managedResourceInstanceSubgraphDeleteOrForgetThenCreate(plannedChange, providerClientRef, dependencyWaiter)
	case plans.CreateThenDelete:
		finalResultRef = b.managedResourceInstanceSubgraphCreateThenDelete(plannedChange, providerClientRef, dependencyWaiter)
	default:
		// We should not get here: the cases above should cover every action
		// that [planGlue.planDesiredManagedResourceInstance] can possibly
		// produce.
		panic(fmt.Sprintf("unsupported change action %s for %s", plannedChange.Action, plannedChange.Addr))
	}

	closeDependencyAfter(finalResultRef)
	return finalResultRef
}

func (b *execGraphBuilder) managedResourceInstanceSubgraphCreate(
	plannedChange *plans.ResourceInstanceChange,
	providerClientRef execgraph.ResultRef[*exec.ProviderClient],
	waitFor execgraph.AnyResultRef,
) execgraph.ResourceInstanceResultRef {
	instAddrRef, _ := b.managedResourceInstanceChangeAddrAndPriorStateRefs(plannedChange)
	plannedValRef := b.lower.ConstantValue(plannedChange.After)
	desiredInstRef := b.lower.ResourceInstanceDesired(instAddrRef, waitFor)
	return b.managedResourceInstanceSubgraphPlanAndApply(
		desiredInstRef,
		execgraph.NilResultRef[*exec.ResourceInstanceObject](),
		plannedValRef,
		providerClientRef,
	)
}

func (b *execGraphBuilder) managedResourceInstanceSubgraphDelete(
	plannedChange *plans.ResourceInstanceChange,
	providerClientRef execgraph.ResultRef[*exec.ProviderClient],
) execgraph.ResourceInstanceResultRef {
	_, priorStateRef := b.managedResourceInstanceChangeAddrAndPriorStateRefs(plannedChange)
	plannedValRef := b.lower.ConstantValue(plannedChange.After)
	return b.managedResourceInstanceSubgraphPlanAndApply(
		execgraph.NilResultRef[*eval.DesiredResourceInstance](),
		priorStateRef,
		plannedValRef,
		providerClientRef,
	)
}

func (b *execGraphBuilder) managedResourceInstanceSubgraphUpdate(
	plannedChange *plans.ResourceInstanceChange,
	providerClientRef execgraph.ResultRef[*exec.ProviderClient],
	waitFor execgraph.AnyResultRef,
) execgraph.ResourceInstanceResultRef {
	instAddrRef, priorStateRef := b.managedResourceInstanceChangeAddrAndPriorStateRefs(plannedChange)
	plannedValRef := b.lower.ConstantValue(plannedChange.After)
	desiredInstRef := b.lower.ResourceInstanceDesired(instAddrRef, waitFor)
	return b.managedResourceInstanceSubgraphPlanAndApply(
		desiredInstRef,
		priorStateRef,
		plannedValRef,
		providerClientRef,
	)
}

// managedResourceInstanceSubgraphPlanAndApply deals with the simple case
// of "create a final plan and then apply it" that is shared across all of the
// "straightforward" change actions create, update, and delete, but not for
// the more complicated ones involving multiple primitive actions that need
// to be carefully coordinated with each other.
func (b *execGraphBuilder) managedResourceInstanceSubgraphPlanAndApply(
	desiredInstRef execgraph.ResultRef[*eval.DesiredResourceInstance],
	priorStateRef execgraph.ResourceInstanceResultRef,
	plannedValRef execgraph.ResultRef[cty.Value],
	providerClientRef execgraph.ResultRef[*exec.ProviderClient],
) execgraph.ResourceInstanceResultRef {
	finalPlanRef := b.lower.ManagedFinalPlan(
		desiredInstRef,
		priorStateRef,
		plannedValRef,
		providerClientRef,
	)
	return b.lower.ManagedApply(
		finalPlanRef,
		execgraph.NilResultRef[*exec.ResourceInstanceObject](),
		providerClientRef,
		b.lower.Waiter(), // nothing to wait for
	)
}

func (b *execGraphBuilder) managedResourceInstanceSubgraphForget(
	plannedChange *plans.ResourceInstanceChange,
	providerClientRef execgraph.ResultRef[*exec.ProviderClient],
) execgraph.ResourceInstanceResultRef {
	// TODO: Add a new execgraph opcode ManagedForget and use that here.
	panic("execgraph for Forget not yet implemented")
}

func (b *execGraphBuilder) managedResourceInstanceSubgraphDeleteOrForgetThenCreate(
	plannedChange *plans.ResourceInstanceChange,
	providerClientRef execgraph.ResultRef[*exec.ProviderClient],
	waitFor execgraph.AnyResultRef,
) execgraph.ResourceInstanceResultRef {
	if plannedChange.Action == plans.ForgetThenCreate {
		// TODO: Implement this action too, which is similar but with the
		// "delete" let replaced with something like what
		// managedResourceInstanceSubgraphForget would generate.
		panic("execgraph for ForgetThenCreate not yet implemented")
	}

	// This has much the same _effect_ as the separate delete and create
	// actions chained together, but we arrange the operations in such a
	// way that the delete leg can't start unless the desired state is
	// successfully evaluated.
	instAddrRef, priorStateRef := b.managedResourceInstanceChangeAddrAndPriorStateRefs(plannedChange)
	plannedValRef := b.lower.ConstantValue(plannedChange.After)
	desiredInstRef := b.lower.ResourceInstanceDesired(instAddrRef, waitFor)

	// We plan both the create and destroy parts of this process before we
	// make any real changes, to reduce the risk that we'll be left in a
	// partially-applied state where neither object exists. (Though of course
	// that's always possible, if the "create" step fails at apply.)
	createPlanRef := b.lower.ManagedFinalPlan(
		desiredInstRef,
		execgraph.NilResultRef[*exec.ResourceInstanceObject](),
		plannedValRef,
		providerClientRef,
	)
	destroyPlanRef := b.lower.ManagedFinalPlan(
		execgraph.NilResultRef[*eval.DesiredResourceInstance](),
		priorStateRef,
		b.lower.ConstantValue(cty.NullVal(
			// TODO: is this okay or do we need to use the type constraint derived from the schema?
			// The two could differ for resource types that have cty.DynamicPseudoType
			// attributes, like in kubernetes_manifest from the hashicorp/kubernetes provider,
			// where here we'd capture the type of the current manifest instead of recording
			// that the manifest's type is unknown. However, we don't typically fuss too much
			// about the exact type of a null, so this is probably fine.
			plannedChange.After.Type(),
		)),
		providerClientRef,
	)
	destroyResultRef := b.lower.ManagedApply(
		destroyPlanRef,
		execgraph.NilResultRef[*exec.ResourceInstanceObject](),
		providerClientRef,
		b.lower.Waiter(createPlanRef), // wait for successful planning of the create step
	)
	createResultRef := b.lower.ManagedApply(
		createPlanRef,
		execgraph.NilResultRef[*exec.ResourceInstanceObject](),
		providerClientRef,
		b.lower.Waiter(destroyResultRef),
	)

	return createResultRef
}

func (b *execGraphBuilder) managedResourceInstanceSubgraphCreateThenDelete(
	plannedChange *plans.ResourceInstanceChange,
	providerClientRef execgraph.ResultRef[*exec.ProviderClient],
	waitFor execgraph.AnyResultRef,
) execgraph.ResourceInstanceResultRef {
	// TODO: Add a new execgraph opcode ManagedForget and use that here.
	panic("execgraph for cbd replace actions not yet implemented")
}

func (b *execGraphBuilder) managedResourceInstanceChangeAddrAndPriorStateRefs(
	plannedChange *plans.ResourceInstanceChange,
) (
	newAddr execgraph.ResultRef[addrs.AbsResourceInstance],
	priorState execgraph.ResourceInstanceResultRef,
) {
	if plannedChange.Action == plans.Create {
		// For a create change there is no prior state at all, but we still
		// need the new instance address.
		newAddrRef := b.lower.ConstantResourceInstAddr(plannedChange.Addr)
		return newAddrRef, execgraph.NilResultRef[*exec.ResourceInstanceObject]()
	}
	prevAddrRef := b.lower.ConstantResourceInstAddr(plannedChange.PrevRunAddr)
	if plannedChange.DeposedKey != states.NotDeposed {
		// We need to use a different operation to access deposed objects.
		prevAddrRef := b.lower.ConstantResourceInstAddr(plannedChange.PrevRunAddr)
		dkRef := b.lower.ConstantDeposedKey(plannedChange.DeposedKey)
		stateRef := b.lower.ManagedAlreadyDeposed(prevAddrRef, dkRef)
		return execgraph.NilResultRef[addrs.AbsResourceInstance](), stateRef
	}
	newAddrRef := b.lower.ConstantResourceInstAddr(plannedChange.Addr)
	if !plannedChange.PrevRunAddr.Equal(plannedChange.Addr) {
		// If the address is changing then we need to use the "change address"
		// operation instead of just reading te prior state.
		stateRef := b.lower.ManagedChangeAddr(prevAddrRef, newAddrRef)
		return newAddrRef, stateRef
	}
	// In all other cases we just take the prior state directly out of the
	// prior state, without any special behavior.
	stateRef := b.lower.ResourceInstancePrior(prevAddrRef)
	return newAddrRef, stateRef
}
