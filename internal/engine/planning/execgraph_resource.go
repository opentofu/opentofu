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
	additionalStateDependencies addrs.Set[addrs.AbsResourceInstance],
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

	// subgraphs tracks the connection points for the subgraphs of each resource
	// instance object so that we can add additional edges between these
	// subgraphs in the second loop below.
	subgraphs := addrs.MakeMap[addrs.AbsResourceInstanceObject, resourceInstanceObjectSubgraph]()

	// resultRefs tracks the execgraph result reference for each resource
	// instance object, populated gradually as we build it out. We track this
	// separately from the objects in "subgraphs" because we will potentially
	// add synthetic new entries to this map to read the prior state for
	// anything that isn't otherwise being changed as part of this plan.
	resultRefs := addrs.MakeMap[addrs.AbsResourceInstanceObject, execgraph.ResourceInstanceResultRef]()

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
		subgraphs.Put(addr, subgraph)

		if addr.IsCurrent() && subgraph.valueRef != nil {
			resultRefs.Put(addr, subgraph.valueRef)
			b.lower.SetResourceInstanceFinalStateResult(addr.InstanceAddr, subgraph.valueRef)
		}
	}

	// Now we'll add explicit dependencies between the subgraphs we just created
	// for the resource instance object changes. Any object that has a planned
	// change should already have entries in addConfigDeps/addDeleteDeps where
	// appropriate, but we will need to add prior-state-reading stubs for
	// any object that isn't being changed but is a dependency for something
	// that is changing.
	for _, addr := range objAddrs {
		subgraph := subgraphs.Get(addr)
		if subgraph.addDesiredDep != nil {
			for dependency := range objs.ConfigDependencies(addr) {
				subgraph.addDesiredDep(ensureResourceInstanceObjectResultRef(dependency, resultRefs, b))
			}
			for dependency := range objs.StateDependencies(addr) {
				subgraph.addDesiredDep(ensureResourceInstanceObjectResultRef(dependency, resultRefs, b))
			}
		}
		if subgraph.addOrphanDep != nil {
			for dependent := range objs.ConfigDependents(addr) {
				dependentSubgraph := subgraphs.Get(dependent)
				if ref := dependentSubgraph.deletionRef; ref != nil {
					subgraph.addOrphanDep(ref)
				} else if ref, ok := resultRefs.GetOk(dependent); ok {
					// If there's no deletion ref but the dependent still has
					// a value ref (e.g. if the dependent is only being created
					// or updated) then we wait until its create/update step
					// has completed before deleting.
					subgraph.addOrphanDep(ref)
					// TODO: The old runtime's equivalent of this behavior has a
					// hackily-implemented extra rule where it checks if
					// adding this edge would create a cycle and skips adding
					// it if so. Do we need to replicate that here? The
					// commentary on the function that implements it
					// ([DestroyEdgeTransformer.tryInterProviderDestroyEdge])
					// explains that it's needed because, when applying a
					// destroy-mode plan, provider instances are allowed to refer
					// to a resource instance that's being destroyed and expect
					// that to take values from the prior state, and so we'll
					// tackle this question when we add support for destroy-mode
					// planning (which seems likely to have its own separate
					// execgraph-building logic just because the rules are quite
					// different for that planning mode).
				}
			}
			for dependent := range objs.StateDependents(addr) {
				dependentSubgraph := subgraphs.Get(dependent)
				if ref := dependentSubgraph.deletionRef; ref != nil {
					subgraph.addOrphanDep(ref)
				} else if ref, ok := resultRefs.GetOk(dependent); ok {
					subgraph.addOrphanDep(ref)
				}
			}
		}
	}

	// There are some scenarios in which we require resourceInstances outside of the execution graph (ex: root output values)
	// Here we ensure that they make it into the execution graph and don't hit the "missing resource instance"
	// ephemeral detection logic.
	for _, dependency := range additionalStateDependencies {
		ensureResourceInstanceObjectResultRef(dependency.CurrentObject(), resultRefs, b)
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
		b.lower.SetResourceInstanceFinalStateResult(addr.InstanceAddr, resultRef)
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

// resourceInstanceObjectSubgraph represents the external connection points of a
// previously-built resource instance object subgraph so that we can
// subsequently create dependency edges between the subgraphs.
type resourceInstanceObjectSubgraph struct {
	// valueRef is the result reference for the final state for this resource
	// instance object that should be used to satisfy expressions that refer
	// to this resource instance object.
	//
	// For objects that are just being deleted (i.e. "orphan" or "deposed"
	// objects for managed resource instances), this actually refers to the
	// prior state. This is a quirky but important special case to make
	// it possible to apply a "destroy-mode" plan where an ephemeral object
	// such as a provider instance depends on a resource instance, in which
	// case the ephemeral object gets configured based on the (refreshed)
	// prior state rather than the configured object that is expected to be
	// deleted during the apply phase.
	valueRef execgraph.ResourceInstanceResultRef

	// deletionRef is a result reference which resolves once any existing
	// remote object has been deleted.
	//
	// This is nil for objects whose planned actions don't include deletion
	// at all.
	deletionRef execgraph.AnyResultRef

	// addDesiredDep and addOrphanDep each add an additional dependency to
	// one of the operations in the subgraph.
	//
	// addDesiredDep adds a dependency of whatever operation causes there
	// to be an object matching the desired state, whether that be creating
	// or updating. It is nil for resource instance objects that have no
	// desired state, i.e. when they are just being deleted.
	//
	// addOrphanDep adds a dependency of whatever operation causes an object
	// to be deleted because it's no longer desired, which could either be
	// the solitary operation to delete something that is "orphaned" (present
	// in prior state but not desired state) or could be the delete leg of
	// a replace operation. It is nil for resource instance objects where no
	// deletion is needed, i.e. when they are only being created or updated.
	addDesiredDep, addOrphanDep func(execgraph.AnyResultRef)
}
