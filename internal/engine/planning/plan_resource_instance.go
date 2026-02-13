// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
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
	// InstanceAddr is the address of the resource instance this object
	// belongs to.
	InstanceAddr addrs.AbsResourceInstance

	// DeposedKey is the deposed key of the object, or [states.NotDeposed]
	// if this is the "current" object for the resource instance.
	DeposedKey states.DeposedKey

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

	// Dependencies is the set of all resource instances that this object's
	// resource instance depends on either directly or indirectly.
	//
	// Note that this describes the dependencies between the resource instances
	// as declared or implied in configuration, NOT the ordering requirements
	// of the specific change in the PlannedChange field. In particular, if
	// the planned action is [plans.Delete] then this field must still record
	// the normal dependencies of the resource instances whose object is being
	// destroyed and NOT the "inverted" dependencies that would be reflected in
	// the final execution graph.
	//
	// For orphan or deposed objects which therefore appear only in state and
	// have no current configured dependencies, this should describe all of
	// the resource instances in the prior state that the state object was
	// recorded as depending on, instead of dependencies detected through the
	// configuration.
	Dependencies addrs.Set[addrs.AbsResourceInstance]
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
