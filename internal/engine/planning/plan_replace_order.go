// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
)

// findEffectiveReplaceOrders analyzes the given graph of resource instance
// objects to decide the final effective "replace order" for each resource
// instance object.
//
// Specifically, any object whose initial replace order is [replaceAnyOrder]
// will have its effective order set to either [replaceDestroyThenCreate] or
// [replaceCreateThenDestroy], depending on whether they are in dependency
// chains with objects those initial replace order was
// [replaceCreateThenDestroy]. All objects in a chain of dependencies are
// required to have the same replace order.
//
// The second return value is a set of addresses of objects which depend on
// themselves either directly or indirectly, which should be impossible if the
// graph was constructed correctly. If that set contains any elements then the
// map of effective replace orders is likely to be incomplete.
//
// This function currently assumes that all of the provided objects have their
// initial replace order set to either [replaceAnyOrder] or
// [replaceCreateThenDestroy]. If any of the given objects have the initial
// order [replaceDestroyThenCreate] then this function will panic; that
// replace order is used only as the _effective_ replace order for any object
// that isn't chained with an object whose initial order is
// [replaceCreateThenDestroy]. This models the current constraints of the
// surface language where "create_before_destroy = true" is treated as
// [replaceCreateThenDestroy] and everything else is treated as
// [replaceAnyOrder].
func findEffectiveReplaceOrders(objs *resourceInstanceObjects) (addrs.Map[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder], addrs.Set[addrs.AbsResourceInstanceObject]) {
	orders := addrs.MakeMap[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]()
	selfDeps := addrs.MakeSet[addrs.AbsResourceInstanceObject]()

	// This initial implementation is pretty simplistic: we just visit every
	// object in an arbitrary order and then visit each of its dependencies
	// and dependents in an arbitrary order until either we find one that
	// requires "create then destroy" or until we run out of objects to check.
	//
	// Maybe later we'll devise a cleverer algorithm for this which doesn't
	// involve revisiting the same objects quite as much. For now our only
	// minor optimization is to stop as soon as we find the first neighbor
	// with [replaceCreateThenDestroy].

Objects:
	for currentInst, currentObj := range objs.All() {
		if currentObj.ReplaceOrder == replaceCreateThenDestroy {
			// Easy case: this one is definitely create-then-destroy.
			orders.Put(currentInst, replaceCreateThenDestroy)
			continue
		}

		for otherInst := range objs.DependenciesAndDependents(currentInst) {
			if otherInst.Equal(currentInst) {
				// We've found a self-dependency problem, so we'll record
				// it but continue anyway because the rest of this algorithm
				// can tolerate that sitution for now.
				selfDeps.Add(currentInst)
				continue
			}

			if currentObj.ReplaceOrder != replaceAnyOrder && currentObj.ReplaceOrder != replaceCreateThenDestroy {
				panic(fmt.Sprintf("%s has invalid initial replace order %s", currentInst, currentObj.ReplaceOrder))
			}

			// If we've already recorded a decision for this one then we'll
			// prefer to use that decision. At this point in the process that
			// decision can only be [replaceCreateThenDestroy], because we
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
			if otherObj.ReplaceOrder == replaceCreateThenDestroy {
				orders.Put(currentInst, replaceCreateThenDestroy)
			}
		}
	}

	// Now we'll make a followup pass and just set everything we didn't already
	// decide to replaceDestroyThenCreate, which is the default.
	for currentInst := range objs.All() {
		if !orders.Has(currentInst) {
			orders.Put(currentInst, replaceDestroyThenCreate)
		}
	}

	return orders, selfDeps
}

// resourceInstanceReplaceOrder represents the constraint, if any, for what
// order the create and destroy steps of a "replace" action must happen in.
type resourceInstanceReplaceOrder int

//go:generate go tool golang.org/x/tools/cmd/stringer -type=resourceInstanceReplaceOrder -trimprefix=replace

const (
	// replaceAnyOrder means that it's okay to use either order, in which
	// case the associated resource instance will just follow the prevailing
	// order chosen by its upstream and downstream dependencies.
	//
	// It isn't possible for conflicting replace orders to coexist in the
	// same chain of dependent resource instances because that would mean there
	// is no valid order to perform the steps in, and so we rely on the
	// assumption that most resource instances begin without any constraint
	// and then just follow whatever order is required to satisfy the needs
	// of their neighbors.
	replaceAnyOrder resourceInstanceReplaceOrder = iota

	// replaceCreateThenDestroy represents that a replacement object must be
	// created before destroying the previous object.
	replaceCreateThenDestroy

	// replaceDestroyThenCreate represents that the previous object must be
	// destroyed before creating its replacement, such as if both objects
	// would try to occupy the same unique object name and so cannot coexist
	// at the same time.
	//
	// This is the default resolution if all dependencies in a chain start
	// off as [replaceAnyOrder].
	replaceDestroyThenCreate
)

// ChangeAction returns the [plans.Action] corresponding to the reciever,
// or panics if the reciever is [replaceAnyOrder] because that value represents
// that we haven't yet decided which action to use.
//
// This should typically be used only on values taken from the result of a
// call to [findEffectiveReplaceOrders], where all resource instance objects
// are expected to have a definitive effective replace order.
func (o resourceInstanceReplaceOrder) ChangeAction() plans.Action {
	switch o {
	case replaceCreateThenDestroy:
		return plans.CreateThenDelete
	case replaceDestroyThenCreate:
		return plans.DeleteThenCreate
	default:
		panic(fmt.Errorf("no change action for undecided replace order"))
	}
}
