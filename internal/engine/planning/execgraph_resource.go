// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
	"github.com/opentofu/opentofu/internal/plans"
)

////////////////////////////////////////////////////////////////////////////////
// This file contains methods of [execGraphBuilder] that are related to the
// parts of an execution graph that deal with resource instances of all modes.
////////////////////////////////////////////////////////////////////////////////

// AddResourceInstanceObjectSubgraphs adds all of the execution graph items
// needed to apply the planned changes for the given resource instance objects,
// including the operations required for the provider instances that those
// resource instances belong to.
func (b *execGraphBuilder) AddResourceInstanceObjectSubgraphs(
	objs *resourceInstanceObjects,
	effectiveReplaceOrders addrs.Map[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder],
) {
	// TODO: We don't currently have any unit tests for this function. If this
	// survives into a shipping version of the planning engine then we should
	// write unit tests, and until then we should aim to keep this function
	// self-contained so that it _could_ be unit tested in isolation from the
	// rest of the planning engine.

	// TODO: With the earlier incarnation of execgraph building we assumed that
	// cycles in the execution graph were basically impossible because in all
	// cases except provider close we were always adding dependency before
	// dependent. This new model instead adds all of the subggraphs first and
	// then adds the explicit dependencies between them afterwards, so this
	// _could_ produce a cyclic graph if the input isn't valid. Can we do
	// something in here to detect cycles during the graph-building process,
	// or do we instead need a post-hoc validate step which applies Tarjan's
	// Strongly Connected Components algorithm to the execution graph?

	// resultRefs tracks the execgraph result reference for each resource
	// instance object, populated gradually as we build it out.
	resultRefs := addrs.MakeMap[addrs.AbsResourceInstanceObject, execgraph.ResourceInstanceResultRef]()
	// deletionRefs is like resultRefs except that it tracks the result of
	// a deletion step for each object. There's only an entry in this table
	// for objects whose subgraphs involve a deletion.
	deletionRefs := addrs.MakeMap[addrs.AbsResourceInstanceObject, execgraph.ResourceInstanceResultRef]()

	// addConfigDeps and addDeleteDeps both track functions we can use to add
	// additional dependencies to operations in the execution subgraphs of
	// different resource instance objects.
	//
	// addConfigDeps callbacks are for operations that must complete before
	// evaluating the configuration for an object, and so this captures the
	// relevant dependencies of each object.
	//
	// addDeleteDeps callbacks are for operations that must complete before
	// applying a "delete" plan for the object, and so these represent the
	// "reverse dependencies" between deleting things so that they get destroyed
	// in "inside out" dependency order.
	//
	// Not all resource instance objects will have elements in both of these
	// maps. For example, an addDeleteDeps entry is present only if the
	// execution subgraph for an object includes a ManagedApply operation
	// for a "delete" plan.
	addConfigDeps := addrs.MakeMap[addrs.AbsResourceInstanceObject, func(execgraph.AnyResultRef)]()
	addDeleteDeps := addrs.MakeMap[addrs.AbsResourceInstanceObject, func(execgraph.AnyResultRef)]()

	// We pre-sort the keys here because that causes our execution graph
	// operations to be in a deterministic order, for easier unit testing and
	// easier reading of debug output.
	objAddrs := sortedResourceInstanceObjectAddrKeys(objs.All())

	// First we'll insert separate subgraphs for each resource instance object
	// that has a planned action, without putting any explicit dependency
	// edges between them yet.
	//
	// We'll insert the explicit dependency edges between the subgraphs in a
	// separate loop afterwards, along with any needed prior state operations
	// for objects that aren't changing.
	for _, addr := range objAddrs {
		obj := objs.Get(addr)
		plannedChange := obj.PlannedChange
		if plannedChange == nil {
			// For this first loop we only care about objects that have planned
			// changes. We'll fill in the subset of objects that aren't changing
			// afterwards only if at least one object that _is_ changing depends
			// on them.
			continue
		}

		subgraph := b.resourceInstanceChangeSubgraph(
			plannedChange,
			effectiveReplaceOrders.Get(addr),
		)

		// We'll use these two add*Dep functions in the second loop below as
		// we fill in all of the explicit dependencies caused by expressions
		// in the configuration.
		if subgraph.addConfigDep != nil {
			addConfigDeps.Put(addr, subgraph.addConfigDep)
		}
		if subgraph.addDeleteDep != nil {
			deletionRefs.Put(addr, subgraph.deletionRef)
			addDeleteDeps.Put(addr, subgraph.addDeleteDep)
		}

		if addr.IsCurrent() && subgraph.valueRef != nil {
			resultRefs.Put(addr, subgraph.valueRef)
			b.SetResourceInstanceFinalStateResult(addr.InstanceAddr, subgraph.valueRef)
		}
	}

	// Now we'll add explicit dependencies between the subgraphs we just created
	// for the resource instance object changes. Any object that has a planned
	// change should already have entries in addConfigDeps/addDeleteDeps where
	// appropriate, but we will need to add prior-state-reading stubs for
	// any object that isn't being changed but is a dependency for something
	// that is changing.
	for _, addr := range objAddrs {
		if addConfigDep, ok := addConfigDeps.GetOk(addr); ok {
			for dependency := range objs.ConfigDependencies(addr) {
				addConfigDep(ensureResourceInstanceObjectResultRef(dependency, resultRefs, b))
			}
			// FIXME: For now we're also treating the state dependencies just
			// like config dependencies, which isn't quite right but is a
			// temporary placeholder while we're still wiring in the separation
			// between state and config dependencies.
			for dependency := range objs.StateDependencies(addr) {
				addConfigDep(ensureResourceInstanceObjectResultRef(dependency, resultRefs, b))
			}
		}
		if addDeleteDep, ok := addDeleteDeps.GetOk(addr); ok {
			for dependent := range objs.ConfigDependents(addr) {
				if ref, ok := deletionRefs.GetOk(dependent); ok {
					addDeleteDep(ref)
				} else if ref, ok := resultRefs.GetOk(dependent); ok {
					// If there's no deletion ref but the dependent still has
					// a value ref (e.g. if the dependent is only being created
					// or updated) then we wait until its create/update step
					// has completed before deleting.
					addDeleteDep(ref)
					// TODO: The old runtime's equivalent of this behavior has a
					// hackily-implemented extra rule where it checks if
					// adding this edge would create a cycle and skips adding
					// it if so. Do we need to replicate that here? The
					// commentary on the function that implements it
					// ([DestroyEdgeTransformer.tryInterProviderDestroyEdge])
					// explains that it's needed because when applying a
					// destroy-mode plan provider instances are allowed to refer
					// to a resource instance that's being destroyed and expect
					// that to take values from the prior state, and so we'll
					// tackle this question when we add support for destroy-mode
					// planning (which seems likely to have its own separate
					// execgraph-building logic just because the rules are quite
					// different for that planning mode).
				}
			}
			// FIXME: For now we're also treating the state dependencies just
			// like config dependencies, which isn't quite right but is a
			// temporary placeholder while we're still wiring in the separation
			// between state and config dependencies.
			for dependent := range objs.StateDependents(addr) {
				if ref, ok := deletionRefs.GetOk(dependent); ok {
					addDeleteDep(ref)
				} else if ref, ok := resultRefs.GetOk(dependent); ok {
					addDeleteDep(ref)
				}
			}
		}
	}
}

func ensureResourceInstanceObjectResultRef(addr addrs.AbsResourceInstanceObject, knownResults addrs.Map[addrs.AbsResourceInstanceObject, execgraph.ResourceInstanceResultRef], b *execGraphBuilder) execgraph.ResourceInstanceResultRef {
	if existing, ok := knownResults.GetOk(addr); ok {
		return existing
	}
	// If we don't already have an existing result then this is an object
	// that doesn't have any planned changes, so and we'll just provide
	// a minimum subgraph for it that only involves reading its prior state.
	var resultRef execgraph.ResourceInstanceResultRef
	if addr.IsCurrent() {
		resultRef = b.lower.ResourceInstancePrior(b.lower.ConstantResourceInstAddr(addr.InstanceAddr), nil)
		b.SetResourceInstanceFinalStateResult(addr.InstanceAddr, resultRef)
	} else {
		resultRef = b.lower.ManagedAlreadyDeposed(b.lower.ConstantResourceInstAddr(addr.InstanceAddr), b.lower.ConstantDeposedKey(addr.DeposedKey))
	}
	knownResults.Put(addr, resultRef)
	return resultRef
}

func (b *execGraphBuilder) resourceInstanceChangeSubgraph(
	change *plans.ResourceInstanceChange,
	effectiveReplaceOrder resourceInstanceReplaceOrder,
) resourceInstanceObjectSubgraph {
	resourceMode := change.Addr.Resource.Resource.Mode
	switch resourceMode {
	case addrs.ManagedResourceMode:
		return b.ManagedResourceInstanceSubgraph(change, effectiveReplaceOrder)

	// TODO: DataResourceMode, and possibly also EphemeralResourceMode if
	// we decide to handle those as "changes" (but it's currently looking
	// like they would be better handled in some other special way, since
	// they don't "change" in the same sense that other modes do.)
	default:
		// We should not get here because the above should cover all modes that
		// the earlier planning pass could possibly plan changes for.
		panic(fmt.Sprintf("can't build resource instance change subgraph for unexpected resource mode %s", resourceMode))
	}
}

// SetResourceInstanceFinalStateResult records which result should be treated
// as the "final state" for the given resource instance, for purposes such as
// propagating the result value back into the evaluation system to allow
// downstream expressions to derive from it.
//
// Only one call is allowed per distinct [addrs.AbsResourceInstance] value. If
// two callers try to register for the same address then the second call will
// panic.
func (b *execGraphBuilder) SetResourceInstanceFinalStateResult(addr addrs.AbsResourceInstance, result execgraph.ResourceInstanceResultRef) {
	b.mu.Lock()
	b.lower.SetResourceInstanceFinalStateResult(addr, result)
	b.mu.Unlock()
}

// resourceInstanceObjectSubgraph represents the external connection points of a
// previously-built resource instance object subgraph so that we can
// subsequently create dependency edges between the subgraphs.
type resourceInstanceObjectSubgraph struct {
	// valueRef and deletionRef are the result references for whatever
	// operations in the subgraph represent the result of creating/updating
	// and of deleting the resource instance respectively.
	//
	// These are nil for subgraphs that don't include a relevant result.
	valueRef, deletionRef execgraph.ResourceInstanceResultRef

	// addConfigDep and addDeleteDep each add an additional dependency to
	// one of the operations in the subgraph.
	//
	// addConfigDep adds a dependency of whatever operation accesses the
	// configuration for the resource instance object, and so delays the
	// evaluation of
	// the configuration.
	//
	// addDeleteDep adds a dependency of whatever operation deletes the
	// associated object.
	//
	// Each of these is nil when there is no corresponding operation to add
	// dependencies to.
	addConfigDep, addDeleteDep func(execgraph.AnyResultRef)
}
