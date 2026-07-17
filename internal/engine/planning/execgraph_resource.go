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
			b.lower.SetResourceInstanceFinalStateResult(addr.InstanceAddr, subgraph.valueRef)
		}
	}

	// Now we'll add explicit dependencies between the subgraphs we just created
	// for the resource instance object changes. Any object that has a planned
	// change should already have an entry in "subgraphs", but we will need to
	// add prior-state-reading stubs for any object that isn't being changed but
	// is a dependency for something that is changing.
	//
	// In the following we follow the convention of describing edges between
	// operations for objects A and B, where B depends on A. Exactly how we
	// wire together the subgraphs of these objects depends on whether they
	// each have create/update and delete legs in their subgraphs and whether
	// they use the "create before destroy" ordering for replacing.
	for _, addrB := range objAddrs {
		objB := subgraphs.Get(addrB)

		for addrA := range objs.ConfigDependencies(addrB) {
			objA := ensureResourceInstanceObjectSubgraph(addrA, subgraphs, b)

			// Config-based dependencies are relatively straightforward because
			// they can only possibly constrain ordering of creation-related
			// operations.
			if objA.HasCreateOrUpdateLeg() && objB.HasCreateOrUpdateLeg() {
				objB.CreateOrUpdateDependsOn(objA.CreateOrUpdateCompletionRef())
			}
		}

		for addrA := range objs.StateDependencies(addrB) {
			objA := ensureResourceInstanceObjectSubgraph(addrA, subgraphs, b)

			// The creation-related operations still just get connected in
			// the intuitive order where A would be created before B, as
			// with the config-based dependencies.
			if objA.HasCreateOrUpdateLeg() && objB.HasCreateOrUpdateLeg() {
				// (Note that this can potentially duplicate connections we
				// already made in the ConfigDependencies loop above. We just
				// ignore those redundant dependencies for now, since their
				// overhead is minimal.)
				objB.CreateOrUpdateDependsOn(objA.CreateOrUpdateCompletionRef())
			}
			// If object A is not being deleted then there's nothing else for us
			// to do here.
			if !objA.HasDeleteLeg() {
				continue
			}

			// Unfortunately the rules for the ordering of deletion operations
			// are more complicated because we need to handle various awkward
			// situations, including:
			// - Making sure we connect the subgraphs in a way that respects
			//   the replace order of each object, if either is being replaced.
			// - Dealing with situations where create-only or delete-only
			//   objects interact with those that are being replaced, and so
			//   we might need to e.g. use a create/update result as a
			//   placeholder for an absent delete result.
			//
			// Regardless of the above we can assume that the individual
			// create/delete operations inside each subgraph are already ordered
			// correctly for the selected replace order, and so we only need
			// to worry about the connections _between_ the subgraphs.
			cbdA := effectiveReplaceOrders.Get(addrA) == replaceCreateThenDestroy
			cbdB := effectiveReplaceOrders.Get(addrB) == replaceCreateThenDestroy
			if cbdA {
				// If A is create_before_destroy then we're aiming for an
				// ordering like this:
				//   - Create A
				//   - Create B
				//   - Delete B
				//   - Delete A
				if objB.HasCreateOrUpdateLeg() {
					objA.DeleteDependsOn(objB.CreateOrUpdateCompletionRef())
				}
				if objB.HasDeleteLeg() {
					// In this case we need to connect differently depending
					// on the replace order being used by B, so that we can
					// avoid creating a cycle.
					if cbdB {
						objA.DeleteDependsOn(objB.DeletionRef())
					} else {
						// If A is cbd but B is not then we need to connect
						// this the other way around, to achieve this unusual
						// ordering where A and B's operations are interleaved:
						//   - Delete B
						//   - Create A
						//   - Create B
						//   - Delete A
						if objA.HasCreateOrUpdateLeg() {
							objA.CreateOrUpdateDependsOn(objB.DeletionRef())
						}
					}
				}
			} else if objB.HasDeleteLeg() {
				// For all other combinations, we just use the intuitive
				// rule that B must be deleted before A, because a
				// dependency must outlive objects that depend on it.
				//
				// If both objects also have creation legs (i.e. they are both
				// being replaced) then this combines with the dependency
				// between the two create legs that we added above to produce
				// the following effective order:
				//   - Delete B
				//   - Delete A
				//   - Create A
				//   - Create B
				objA.DeleteDependsOn(objB.DeletionRef())
			}
		}
	}

	// There are some scenarios in which we require resourceInstances outside of the execution graph (ex: root output values)
	// Here we ensure that they make it into the execution graph and don't hit the "missing resource instance"
	// ephemeral detection logic.
	for _, dependency := range additionalStateDependencies {
		ensureResourceInstanceObjectSubgraph(dependency.CurrentObject(), subgraphs, b)
	}
}

func ensureResourceInstanceObjectSubgraph(addr addrs.AbsResourceInstanceObject, subgraphs addrs.Map[addrs.AbsResourceInstanceObject, resourceInstanceObjectSubgraph], b *execGraphBuilder) resourceInstanceObjectSubgraph {
	if existing, ok := subgraphs.GetOk(addr); ok {
		return existing
	}
	// If we don't already have an existing subgraph then this is an object
	// that doesn't have any planned changes, so and we'll just provide
	// a minimum subgraph for it that only involves reading its prior state.
	var resultRef execgraph.ResourceInstanceResultRef
	if addr.IsCurrent() {
		resultRef = b.lower.ResourceInstancePrior(b.lower.ConstantResourceInstAddr(addr.InstanceAddr), nil)
		b.lower.SetResourceInstanceFinalStateResult(addr.InstanceAddr, resultRef)
	} else {
		resultRef = b.lower.ManagedAlreadyDeposed(b.lower.ConstantResourceInstAddr(addr.InstanceAddr), b.lower.ConstantDeposedKey(addr.DeposedKey))
	}
	subgraph := resourceInstanceObjectSubgraph{
		valueRef: resultRef,

		// We intentionally don't populate addDesiredDep here, because we're
		// only reading an object from the prior state and so there's never
		// any need for that to have any dependencies. This also means that
		// [resourceInstanceObjectSubgraph.HasCreateOrUpdateLeg] will return
		// false, so the graph builder will know that there's no need to
		// treat this as an explicit dependency of whatever is depending on
		// it since the evaluator will naturally block until this result
		// is available anyway (in ResourceInstanceDesired for any dependency),
		// so an explicit dependency would be redundant and distracting for
		// humans trying to diagnose misbehavior using the debug repr.
	}
	subgraphs.Put(addr, subgraph)
	return subgraph
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

// The following methods of resourceInstanceObjectSubgraph are just here to help
// make the graph-connecting algorithm in [AddResourceInstanceObjectSubgraphs]
// a little easier to read by hiding some of the details in favor of descriptive
// method names.

func (s *resourceInstanceObjectSubgraph) HasCreateOrUpdateLeg() bool {
	return s.addDesiredDep != nil
}

func (s *resourceInstanceObjectSubgraph) HasDeleteLeg() bool {
	return s.addOrphanDep != nil
}

func (s *resourceInstanceObjectSubgraph) CreateOrUpdateCompletionRef() execgraph.AnyResultRef {
	if !s.HasCreateOrUpdateLeg() {
		// In this case valueRef is sometimes set to the prior state value as
		// a way to resolve some awkward situations in a destroy-mode plan, but
		// it doesn't represent completion in that case.
		return nil
	}
	return s.valueRef
}

func (s *resourceInstanceObjectSubgraph) DeletionRef() execgraph.AnyResultRef {
	return s.deletionRef
}

func (s *resourceInstanceObjectSubgraph) CreateOrUpdateDependsOn(r execgraph.AnyResultRef) {
	if s.addDesiredDep == nil {
		panic("attempt to add creation or updating dependency for object that has no create/update leg")
	}
	s.addDesiredDep(r)
}

func (s *resourceInstanceObjectSubgraph) DeleteDependsOn(r execgraph.AnyResultRef) {
	if s.addOrphanDep == nil {
		panic("attempt to add deletion dependency for object that has no deletion leg")
	}
	s.addOrphanDep(r)
}
