// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exec

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states"
)

// ManagedResourceObjectFinalPlan represents a final plan -- ready to actually
// be applied -- for some managed resource instance object that could be any of
// a current object for a desired resource instance, a current object for
// an orphan resource instance, or a deposed object for any resource
// instance.
//
// Note that for execution graph purposes a "replace" action is always
// represented as two separate "final plans", where the "delete" leg is
// represented by the configuration being null and the "create" leg is
// represented by the prior state being null. This struct type intentionally
// does not carry any information about the identity of the object the
// plan is for because that is implied by the relationships in the graph and
// there should be no assumptions about e.g. there being exactly one final
// plan per resource instance, etc.
type ManagedResourceObjectFinalPlan struct {
	// InstanceAddr and DeposedKey together describe which resource instance
	// object this plan was created for.
	//
	// These are to be used only for recording the new state of the object
	// after applying this plan, and should be treated opaquely. In particular,
	// nothing from these fields should be sent to a provider as part of
	// applying the plan because how we track resource instance objects between
	// rounds is an implementation detail that providers should not rely on so
	// that we can potentially change it in future while staying compatible
	// with existing provider plugins.
	InstanceAddr addrs.AbsResourceInstance
	DeposedKey   states.DeposedKey

	// ResourceType is the resource type of the object this plan is for, as
	// would be understood by the provider that generated this plan.
	ResourceType string

	// ConfigVal is the value representing the configuration for this
	// object, but only if it's a "desired" object. This is always a null
	// value for "orphan" instances and deposed objects, because they have
	// no configuration by definition.
	ConfigVal cty.Value
	// PriorStateVal is the value representing this object in the prior
	// state, or a null value if this object didn't previously exist and
	// is therefore presumably being created.
	PriorStateVal cty.Value
	// PlannedVal is the value returned by the provider when it was asked
	// to produce a plan. This is an approximation of the final result
	// with unknown values as placeholders for anything that won't be known
	// until after the change has been applied.
	PlannedVal cty.Value
	// ProviderPrivate is the raw "private" value that the provider returned
	// in its planning response, which must be sent back to the provider
	// verbatim when applying the plan.
	ProviderPrivate []byte
	// TODO: Anything else we'd need to populate an "ApplyResourceChanges"
	// request to the associated provider.
}

// ResourceInstanceObject associates a [states.ResourceInstanceObjectFull] with
// a resource instance address and optional deposed key.
//
// Objects of this type should be treated as immutable. Use the methods of this
// type to derive new objects when modelling changes.
//
// This is intended to model the idea that an object can move between different
// tracking addresses without being modified: an instance of this type
// represents the object existing at a particular address, with the intention
// that a caller would create a new object of this type whenever an object
// moves between addresses but should not need to change the underlying object
// itself.
//
// If an operation _does_ cause an object to move to a new tracking address then
// it should be designed to take an object of this type as an argument
// representing the starting location and then to return a newly-constructed
// separate object of this type representing the new location, so that the
// change of address is modelled in the data flow between operations rather than
// as global mutable state.
type ResourceInstanceObject struct {
	InstanceAddr addrs.AbsResourceInstance
	DeposedKey   states.DeposedKey

	// State is the object currently associated with the given address.
	State *states.ResourceInstanceObjectFull
}

// IntoCurrent returns a new [ResourceInstanceObject] that has the same
// State as the receiver but has DeposedKey set to [states.NotDeposed].
func (o *ResourceInstanceObject) IntoCurrent() *ResourceInstanceObject {
	return &ResourceInstanceObject{
		InstanceAddr: o.InstanceAddr,
		DeposedKey:   states.NotDeposed,
		State:        o.State,
	}
}

// IntoCurrent returns a new [ResourceInstanceObject] that has the same
// State as the receiver but has DeposedKey set the given value.
//
// This function does not (and cannot) verify that the chosen deposed key is
// unique for the resource instance. It's the caller's responsibility to
// allocate a unique deposed key to use.
func (o *ResourceInstanceObject) IntoDeposed(key states.DeposedKey) *ResourceInstanceObject {
	return &ResourceInstanceObject{
		InstanceAddr: o.InstanceAddr,
		DeposedKey:   key,
		State:        o.State,
	}
}

// IntoCurrent returns a new [ResourceInstanceObject] that has the same
// address information as the receiver but has State set to the given object.
//
// If the given state object is nil then the result is also nil, to represent
// the absense of an object. [ResourceInstanceObject] instances should only
// represent objects that actually exist.
func (o *ResourceInstanceObject) WithNewState(newState *states.ResourceInstanceObjectFull) *ResourceInstanceObject {
	if newState == nil {
		return nil
	}
	return &ResourceInstanceObject{
		InstanceAddr: o.InstanceAddr,
		DeposedKey:   o.DeposedKey,
		State:        newState,
	}
}
