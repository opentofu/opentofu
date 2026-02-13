// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"fmt"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
)

// resourceInstanceObject is the planning engine's internal intermediate
// representation of a resource instance object that's participating in the
// planning process.
//
// Objects of this type form an intermediate "resource instance graph" that
// gathers together all of the information needed to produce the final
// "execution graph" that describes what ought to happen during a subsequent
// apply phase.
type resourceInstanceObject struct {
	// Addr is the unique address of this object.
	//
	// If an object's address is expected to change during the apply phase,
	// such as if it's subject to "moved" blocks, then this returns the expected
	// final address after the changes have been made. The address from the
	// prior state is recorded in the PlannedChange object.
	//
	// If the planned change has action [plans.CreateThenDelete] then the
	// "old" object will be turned into a deposed object for the part of the
	// operation where both new and old object need to exist concurrently. The
	// deposed key for that temporary object is only calculated just in time
	// during the apply phase and so is not recorded anywhere in
	// [resourceInstanceObject].
	Addr addrs.AbsResourceInstanceObject

	// PlannedChange is the change that the planning engine has decided for
	// this resource instance object.
	//
	// If no actual change is needed then this object should still be present
	// but should have its change action set to [plans.NoOp].
	PlannedChange *plans.ResourceInstanceChange

	// ReplaceOrder specifies what order the create and destroy steps of a
	// "replace" must happen in for this object.
	//
	// When returning from one of the planning functions on [planGlue] this
	// should focus on describing only the configured constraints of the
	// specific object in question, using [replaceAnyOrder] if there is no
	// constraint.
	//
	// Subsequent processing elsewhere in the planning engine will decide
	// a final effective constraint to replace any [replaceAnyOrder]
	// constraints, by analyzing the dependency flow between objects.
	//
	// Currently the planning engine is allowed to return only either
	// [replaceAnyOrder] or [replaceCreateThenDestroy] in this field.
	// [replaceDestroyThenCreate] is then inferred automatically for any
	// object that isn't forced to be [replaceCreateThenDestroy] by one of
	// its dependency neighbors.
	ReplaceOrder resourceInstanceReplaceOrder

	// Dependencies is the set of all resource instance objects that this
	// object's resource instance depends on either directly or indirectly.
	//
	// Note that this describes the dependencies between the resource instance
	// objects as declared or implied in configuration, NOT the ordering
	// requirements of the specific change in the PlannedChange field. In
	// particular, if the planned action is [plans.Delete] then this field must
	// still record the normal dependencies of the resource instances whose
	// object is being destroyed and NOT the "inverted" dependencies that would
	// be reflected in the final execution graph.
	//
	// For orphan or deposed objects which therefore appear only in state and
	// have no current configured dependencies, this should describe all of
	// the resource instance objects in the prior state that the state object
	// was recorded as depending on, instead of dependencies detected through
	// the configuration. We assume that the dependencies recorded in the state
	// match what was declared in an earlier version of the configuration.
	Dependencies addrs.Set[addrs.AbsResourceInstanceObject]
}

// resourceInstanceObjects is conceptually a map from
// [addrs.AbsResourceInstanceObject] to [*resourceInstanceObject], but
// it supports concurrent writes and also allows querying the dependency
// relationships between objects in both directions.
type resourceInstanceObjects struct {
	// Must always hold mu while accessing any other fields.
	mu sync.Mutex

	// objects are the resource instance objects that have been added so far.
	objects addrs.Map[addrs.AbsResourceInstanceObject, *resourceInstanceObject]

	// reverseDeps describes the same relationships as in
	// [resourceInstanceObject.Dependencies] but viewed from the opposite
	// direction: the map keys are dependencies of the objects in the map
	// values.
	//
	// We maintain this so that we can efficiently traverse the graph in both
	// directions when performing further analysis.
	reverseDeps addrs.Map[addrs.AbsResourceInstanceObject, addrs.Set[addrs.AbsResourceInstanceObject]]
}

func newResourceInstanceObjects() *resourceInstanceObjects {
	return &resourceInstanceObjects{
		objects:     addrs.MakeMap[addrs.AbsResourceInstanceObject, *resourceInstanceObject](),
		reverseDeps: addrs.MakeMap[addrs.AbsResourceInstanceObject, addrs.Set[addrs.AbsResourceInstanceObject]](),
	}
}

// Put inserts the given resource instance object into the collection.
//
// Resource instance objects are uniquely identified by the addresses
// in [resourceInstanceObject.Addr]. Attempting to add an object with the same
// address as a previously-added object causes a panic, because it suggests a
// bug in the caller.
//
// A [resourceInstanceObject] value must not be modified once it has been passed
// to this method.
func (rios *resourceInstanceObjects) Put(obj *resourceInstanceObject) {
	rios.mu.Lock()
	defer rios.mu.Unlock()

	if rios.objects.Has(obj.Addr) {
		panic(fmt.Sprintf("%s is already tracked in this resourceInstanceObjects collection", obj.Addr))
	}
	rios.objects.Put(obj.Addr, obj)

	// We also update the reverseDeps structure here to ensure that our
	// records of the graph edges are always consistent across both directions.
	for depAddr := range obj.Dependencies.All() {
		if !rios.reverseDeps.Has(depAddr) {
			rios.reverseDeps.Put(depAddr, addrs.MakeSet[addrs.AbsResourceInstanceObject]())
		}
		rios.reverseDeps.Get(depAddr).Add(obj.Addr)
	}
}

// Get returns the resource instance object with the given address, or nil if
// no such object has been added.
//
// The caller must not mutate anything accessible through the returned pointer.
func (rios *resourceInstanceObjects) Get(addr addrs.AbsResourceInstanceObject) *resourceInstanceObject {
	rios.mu.Lock()
	ret := rios.objects.Get(addr)
	rios.mu.Unlock()
	return ret
}

// AllAddrs returns a set of all of the resource instance object addresses known
// to this collection.
func (rios *resourceInstanceObjects) AllAddrs() addrs.Set[addrs.AbsResourceInstanceObject] {
	rios.mu.Lock()
	ret := rios.objects.Keys()
	rios.mu.Unlock()
	return ret
}

// Dependencies returns the addresses of all resource instance objects that the
// resource instance object of the given address depends on.
//
// The caller must not modify the result and must not access it concurrently
// with other calls to methods on the same object.
func (rios *resourceInstanceObjects) Dependencies(of addrs.AbsResourceInstanceObject) addrs.Set[addrs.AbsResourceInstanceObject] {
	rios.mu.Lock()
	defer rios.mu.Unlock()

	obj, ok := rios.objects.GetOk(of)
	if !ok {
		return nil
	}
	return obj.Dependencies
}

// Dependents returns the addresses of all resource instance objects that have
// the resource instance object with the given address as one of their
// dependencies. In other words, this queries the dependencies "backwards".
//
// The caller must not modify the result and must not access it concurrently
// with other calls to methods on the same object.
//
// Note that not all of the returned addresses necessarily match an object
// that can be retrieved using [resourceInstanceObjects.Get]. The collection
// of resource instance objects is populated in a "forward dependency" order,
// and so dependencies are added before their dependents and the dependents
// might not be added at all if the planning process failed partway through.
func (rios *resourceInstanceObjects) Dependendents(of addrs.AbsResourceInstanceObject) addrs.Set[addrs.AbsResourceInstanceObject] {
	rios.mu.Lock()
	defer rios.mu.Unlock()

	ret, ok := rios.reverseDeps.GetOk(of)
	if !ok {
		return nil
	}
	return ret
}

// resourceInstanceReplaceOrder represents the constraint, if any, for what
// order the create and destroy steps of a "replace" action must happen in.
type resourceInstanceReplaceOrder int

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
