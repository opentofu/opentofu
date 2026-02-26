// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
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

	// providerClientRefs, addProviderConfigDeps, and addProviderCloseDeps
	// capture the three values we need to be able to connect a resource
	// instance with its provider instance.
	providerClientRefs := addrs.MakeMap[addrs.AbsProviderInstanceCorrect, execgraph.ResultRef[*exec.ProviderClient]]()
	addProviderConfigDeps := addrs.MakeMap[addrs.AbsProviderInstanceCorrect, func(execgraph.AnyResultRef)]()
	addProviderCloseDeps := addrs.MakeMap[addrs.AbsProviderInstanceCorrect, func(execgraph.AnyResultRef)]()

	// We pre-sort the keys here because that causes our execution graph
	// operations to be in a deterministic order, for easier unit testing and
	// easier reading of debug output.
	objAddrs := sortedResourceInstanceObjectAddrKeys(objs.All())

	// First we'll insert separate subgraphs for each resource instance object
	// that has a planned action, without putting any explicit dependency
	// edges between them yet. This loop also ensures that we have the
	// operations needed for any provider instance at least one object
	// belongs to.
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
		// FIXME: We're currenly keeping the provider instance address in a
		// direct field of resourceInstanceObject instead of as part of the
		// plannedChange because we want to use our "correct" provider instance
		// address type. The documented rules for this field are that we expect
		// it to be valid when and only when obj.PlannedChange is not nil.
		providerInstAddr := obj.ProviderInst

		providerClientRef, ok := providerClientRefs.GetOk(providerInstAddr)
		var addProviderCloseDep func(execgraph.AnyResultRef)
		if !ok {
			var addProviderConfigDep func(execgraph.AnyResultRef)
			providerClientRef, addProviderConfigDep, addProviderCloseDep = b.ProviderInstanceSubgraph(providerInstAddr)
			providerClientRefs.Put(providerInstAddr, providerClientRef)
			addProviderConfigDeps.Put(providerInstAddr, addProviderConfigDep)
			addProviderCloseDeps.Put(providerInstAddr, addProviderCloseDep)
		} else {
			addProviderCloseDep = addProviderCloseDeps.Get(providerInstAddr)
		}

		valueRef, deletionRef, addConfigDep, addDeleteDep := b.resourceInstanceChangeSubgraph(
			plannedChange,
			effectiveReplaceOrders.Get(addr),
			providerClientRef,
		)

		// We'll use these two add*Dep functions in the second loop below as
		// we fill in all of the explicit dependencies caused by expressions
		// in the configuration.
		if addConfigDep != nil {
			resultRefs.Put(addr, valueRef)
			addConfigDeps.Put(addr, addConfigDep)
			addProviderCloseDep(valueRef)
		}
		if addDeleteDep != nil {
			deletionRefs.Put(addr, deletionRef)
			addDeleteDeps.Put(addr, addDeleteDep)
			addProviderCloseDep(deletionRef)
		}

		if addr.IsCurrent() {
			b.SetResourceInstanceFinalStateResult(addr.InstanceAddr, valueRef)
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
			for dependency := range objs.Dependencies(addr) {
				addConfigDep(ensureResourceInstanceObjectResultRef(dependency, resultRefs, b))
			}
		}
		if addDeleteDep, ok := addDeleteDeps.GetOk(addr); ok {
			for dependent := range objs.Dependendents(addr) {
				if ref, ok := deletionRefs.GetOk(dependent); ok {
					addDeleteDep(ref)
				}
			}
		}
	}

	// We also need explicit dependency relationships whenever a provider
	// instance's configuration refers to information from a resource instance.
	for _, elem := range addProviderConfigDeps.Elems {
		providerInstAddr := elem.Key
		addConfigDep := elem.Value
		for dependency := range objs.ProviderInstanceDependencies(providerInstAddr) {
			addConfigDep(ensureResourceInstanceObjectResultRef(dependency, resultRefs, b))
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
		resultRef = b.lower.ResourceInstancePrior(b.lower.ConstantResourceInstAddr(addr.InstanceAddr))
	} else {
		resultRef = b.lower.ManagedAlreadyDeposed(b.lower.ConstantResourceInstAddr(addr.InstanceAddr), b.lower.ConstantDeposedKey(addr.DeposedKey))
	}
	knownResults.Put(addr, resultRef)
	return resultRef
}

func (b *execGraphBuilder) resourceInstanceChangeSubgraph(
	change *plans.ResourceInstanceChange,
	effectiveReplaceOrder resourceInstanceReplaceOrder,
	providerClientRef execgraph.ResultRef[*exec.ProviderClient],
) (
	valueRef, deletionRef execgraph.ResourceInstanceResultRef, // reference to the final new value and, if addDeleteDep is not nil, the deletion result
	addConfigDep, addDeleteDep func(execgraph.AnyResultRef), // callbacks to register explicit dependencies, or nil when not relevant
) {
	resourceMode := change.Addr.Resource.Resource.Mode
	switch resourceMode {
	case addrs.ManagedResourceMode:
		return b.ManagedResourceInstanceSubgraph(change, effectiveReplaceOrder, providerClientRef)

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
