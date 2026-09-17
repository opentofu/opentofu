// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/resources"
)

// findEffectiveReplaceOrders analyzes the given graph of resource instance
// objects to decide the final effective "replace order" for each resource
// instance object.
//
// Specifically, any object whose initial replace order is
// [resources.ReplaceAnyOrder] will have its effective order set to either
// [resources.ReplaceDeleteFirst] or [resources.ReplaceCreateFirst], depending
// on whether they are in dependency chains with objects those initial replace
// order was [resources.ReplaceCreateFirst]. All objects in a chain of
// dependencies are required to have a compatible replace order so that the
// execution graph wouuld be acyclic.
//
// The second return value is a set of addresses of objects which depend on
// themselves either directly or indirectly, which should be impossible if the
// graph was constructed correctly. If that set contains any elements then the
// map of effective replace orders is likely to be incomplete.
//
// This function currently assumes that all of the provided objects have their
// initial replace order set to either [resources.ReplaceAnyOrder] or
// [resources.ReplaceCreateFirst]. If any of the given objects have the initial
// order [resources.ReplaceDeleteFirst] then this function will panic; that
// replace order is used only as the _effective_ replace order for any object
// that isn't chained with an object whose initial order is
// [resources.ReplaceCreateFirst]. This models the current constraints of the
// surface language where "create_before_destroy = true" is treated as
// [resources.ReplaceCreateFirst] and everything else is treated as
// [resources.ReplaceAnyOrder].
func findEffectiveReplaceOrders(objs *resourceInstanceObjects) (addrs.Map[addrs.AbsResourceInstanceObject, resources.ReplaceOrder], addrs.Set[addrs.AbsResourceInstanceObject]) {
	orders := addrs.MakeMap[addrs.AbsResourceInstanceObject, resources.ReplaceOrder]()
	selfDeps := addrs.MakeSet[addrs.AbsResourceInstanceObject]()

	// This initial implementation is pretty simplistic: we just visit every
	// object in an arbitrary order and then visit each of its dependencies
	// and dependents in an arbitrary order until either we find one that
	// requires "create then destroy" or until we run out of objects to check.
	//
	// Maybe later we'll devise a cleverer algorithm for this which doesn't
	// involve revisiting the same objects quite as much. For now our only
	// minor optimization is to stop as soon as we find the first neighbor
	// with [resources.ReplaceCreateFirst].

Objects:
	for currentInst, currentObj := range objs.All() {
		if currentObj.ReplaceOrder == resources.ReplaceCreateFirst {
			// Easy case: this one is definitely create-then-destroy.
			orders.Put(currentInst, resources.ReplaceCreateFirst)
			continue
		}

		for otherInst := range objs.AllDependents(currentInst) {
			if otherInst.Equal(currentInst) {
				// We've found a self-dependency problem, so we'll record
				// it but continue anyway because the rest of this algorithm
				// can tolerate that sitution for now.
				selfDeps.Add(currentInst)
				continue
			}

			if currentObj.ReplaceOrder != resources.ReplaceAnyOrder && currentObj.ReplaceOrder != resources.ReplaceCreateFirst {
				// FIXME: The configgraph layer is actually currently allowing
				// writing "create_before_destroy = false" to mean "must be
				// deleted first", which is not something the old runtime ever
				// supported and so we should consider whether we actually
				// want to support it. If not then we should reject that at the
				// config layer too, but if so then we need to handle the case
				// where ReplaceOrder is [resources.ReplaceDeleteFirst].
				panic(fmt.Sprintf("%s has invalid initial replace order %s", currentInst, currentObj.ReplaceOrder))
			}

			// If we've already recorded a decision for this one then we'll
			// prefer to use that decision. At this point in the process that
			// decision can only be [resources.ReplaceCreateFirst], because we
			// don't populate any others until after these loops are complete.
			if previous, ok := orders.GetOk(otherInst); ok {
				orders.Put(currentInst, previous)
				continue Objects
			}

			otherObj := objs.Get(otherInst)
			if otherObj == nil {
				// Can potentially happen if the resource instance graph is
				// incomplete, such as if the input config was invalid. In
				// that case we're just making a best effort to finalize a
				// partial plan, so we'll ignore the invalid item.
				continue
			}
			if otherObj.ReplaceOrder == resources.ReplaceCreateFirst {
				orders.Put(currentInst, resources.ReplaceCreateFirst)
			}
		}
	}

	// Now we'll make a followup pass and just set everything we didn't already
	// decide to ReplaceDeleteFirst, which is the default.
	for currentInst := range objs.All() {
		if !orders.Has(currentInst) {
			orders.Put(currentInst, resources.ReplaceDeleteFirst)
		}
	}

	return orders, selfDeps
}

func replaceOrderPlanAction(order resources.ReplaceOrder) plans.Action {
	switch order {
	case resources.ReplaceCreateFirst:
		return plans.CreateThenDelete
	case resources.ReplaceDeleteFirst:
		return plans.DeleteThenCreate
	default:
		panic(fmt.Errorf("no change action for undecided replace order"))
	}
}
