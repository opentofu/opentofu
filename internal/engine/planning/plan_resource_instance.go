// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"fmt"
	"iter"
	"sync"

	"github.com/zclconf/go-cty/cty"

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
	// If no actual change is needed then this object should not be present,
	// and the effective result value should be placed in the PlaceholderValue
	// field. Objects without a planned change are included in the execution
	// graph only if a planned change for another object refers to them, and
	// in that case only as a read from the prior state, and so PlaceholderValue
	// should represent what would be read from the prior state during apply.
	//
	// If the planned action is to "replace" (for managed resource instance
	// objects) and the ReplaceOrder field is set to [replaceAnyOrder] then
	// the planned action can be set to either [plans.DestroyThenCreate] or
	// [plans.CreateThenDestroy] and then will be overridden once the
	// replace order has been finalized in subsequent analysis. If ReplaceOrder
	// is set to a specific value then the planned change's action must
	// immediately agree with the selected order.
	// TODO: As we consider adjusting the plan model to better suit this new
	// runtime, hopefully we can have the plan action initially set to just
	// a generic "replace" and then let ReplaceOrder alone specify which order
	// to use, so that we can avoid this redundancy.
	PlannedChange *plans.ResourceInstanceChange

	// PlaceholderValue is the value that should be used to satisfy references
	// to this object in expressions elsewhere in the configuration when
	// the PlannedChange field contains a nil change.
	//
	// This MUST Be set to a valid value unless the PlannedChange field is
	// populated. If PlannedChange is populated then this field is completely
	// ignored.
	//
	// The placeholder value is a conservative approximation of what we know
	// should definitely match a hypothetical successful plan for this object,
	// so that downstream references can still be typechecked even when
	// their upstreams could not complete planning. This allows us to give as
	// much information as possible about an invalid configuration, but that
	// relies on the placeholder having unknown values in any position where
	// we cannot predict the final result. If we don't know anything at all
	// about the final result then it's valid to use [cty.DynamicVal] here.
	PlaceholderValue cty.Value

	// Provider is the address of the provider the resource type of this
	// object belongs to, and thus the provider whose schema we must use
	// when interpreting the old, new, or placeholder values for this object.
	//
	// This must always be populated, even if we don't know exactly which
	// instance of this provider is responsible for it. If PlannedChange is
	// also populated then the provider instance recorded as being responsible
	// for the change must belong to the same provider recorded here.
	Provider addrs.Provider

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

// ResultValue returns the value that should be sent to the evaluator for
// use in resolving downstream expressions that refer to this resource instance
// object.
//
// If the PlannedChange field is populated then this returns its "After" value.
//
// Otherwise, this returns PlaceholderValue so that downstream planning can
// potentially still proceed based on partial information.
func (rio *resourceInstanceObject) ResultValue() cty.Value {
	if rio.PlannedChange != nil {
		return rio.PlannedChange.After
	}
	if rio.PlaceholderValue != cty.NilVal {
		return rio.PlaceholderValue
	}
	// We should not get here for correctly-constructed objects, but for
	// robustness we'll use cty.DynamicVal as the ultimate placeholder.
	return cty.DynamicVal
}

// resourceInstanceObjects is conceptually a map from
// [addrs.AbsResourceInstanceObject] to [*resourceInstanceObject], but
// it supports concurrent writes and also allows querying the dependency
// relationships between objects in both directions.
//
// Collections of this type and everything inside them should be treated as
// immutable. Use [newResourceInstanceObjectsBuilder] to obtain a temporary
// object for constructing a new objects collection, and then call
// [resourceInstanceObjectsBuilder.Close] to derive the final immutable
// collection from it.
type resourceInstanceObjects struct {
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

// Get returns the resource instance object with the given address, or nil if
// no such object has been added.
//
// The caller must not mutate anything accessible through the returned pointer.
func (rios *resourceInstanceObjects) Get(addr addrs.AbsResourceInstanceObject) *resourceInstanceObject {
	return rios.objects.Get(addr)
}

// AllAddrs returns a sequence over all of the resource instance objects known
// to this collection.
func (rios *resourceInstanceObjects) All() iter.Seq2[addrs.AbsResourceInstanceObject, *resourceInstanceObject] {
	return func(yield func(addrs.AbsResourceInstanceObject, *resourceInstanceObject) bool) {
		for _, elem := range rios.objects.Elems {
			if !yield(elem.Key, elem.Value) {
				return
			}
		}
	}
}

// Dependencies returns the addresses of all resource instance objects that the
// resource instance object of the given address depends on.
//
// The caller must not modify the result and must not access it concurrently
// with other calls to methods on the same object.
func (rios *resourceInstanceObjects) Dependencies(of addrs.AbsResourceInstanceObject) iter.Seq[addrs.AbsResourceInstanceObject] {
	obj := rios.objects.Get(of)
	if obj == nil {
		return func(yield func(addrs.AbsResourceInstanceObject) bool) {}
	}
	return obj.Dependencies.All()
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
func (rios *resourceInstanceObjects) Dependendents(of addrs.AbsResourceInstanceObject) iter.Seq[addrs.AbsResourceInstanceObject] {
	return rios.reverseDeps.Get(of).All()
}

// DependenciesAndDependents is a convenience helper that concatenates together
// the results of both [resourceInstanceObjects.Dependencies] and
// [resourceInstanceObjects.Dependents] into a single flat sequence. It does
// not transform those sequences in any other way.
func (rios *resourceInstanceObjects) DependenciesAndDependents(of addrs.AbsResourceInstanceObject) iter.Seq[addrs.AbsResourceInstanceObject] {
	return func(yield func(addrs.AbsResourceInstanceObject) bool) {
		for addr := range rios.Dependencies(of) {
			if !yield(addr) {
				return
			}
		}
		for addr := range rios.Dependendents(of) {
			if !yield(addr) {
				return
			}
		}
	}
}

// resourceInstanceObjectsBuilder is a wrapper around a
// [resourceInstanceObjects] that allows new objects to be inserted in a
// concurrency-safe way.
type resourceInstanceObjectsBuilder struct {
	mu     sync.Mutex
	result *resourceInstanceObjects
}

func newResourceInstanceObjectsBuilder() *resourceInstanceObjectsBuilder {
	return &resourceInstanceObjectsBuilder{
		result: &resourceInstanceObjects{
			objects:     addrs.MakeMap[addrs.AbsResourceInstanceObject, *resourceInstanceObject](),
			reverseDeps: addrs.MakeMap[addrs.AbsResourceInstanceObject, addrs.Set[addrs.AbsResourceInstanceObject]](),
		},
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
func (b *resourceInstanceObjectsBuilder) Put(obj *resourceInstanceObject) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.result.objects.Has(obj.Addr) {
		panic(fmt.Sprintf("%s is already tracked in this resourceInstanceObjects collection", obj.Addr))
	}
	b.result.objects.Put(obj.Addr, obj)

	// We also update the reverseDeps structure here to ensure that our
	// records of the graph edges are always consistent across both directions.
	for depAddr := range obj.Dependencies.All() {
		if !b.result.reverseDeps.Has(depAddr) {
			b.result.reverseDeps.Put(depAddr, addrs.MakeSet[addrs.AbsResourceInstanceObject]())
		}
		b.result.reverseDeps.Get(depAddr).Add(obj.Addr)
	}
}

func (b *resourceInstanceObjectsBuilder) Close() *resourceInstanceObjects {
	b.mu.Lock()
	ret := b.result
	b.result = nil // this builder can't be used anymore
	b.mu.Unlock()
	return ret
}
