// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exec

import (
	"fmt"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/lang/exprs"
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
	// Addr describes which resource instance object this plan was created for.
	//
	// This is to be used only for recording the new state of the object
	// after applying this plan, and should be treated opaquely. In particular,
	// nothing from these fields should be sent to a provider as part of
	// applying the plan because how we track resource instance objects between
	// rounds is an implementation detail that providers should not rely on so
	// that we can potentially change it in future while staying compatible
	// with existing provider plugins.
	Addr addrs.AbsResourceInstanceObject

	ProviderInstance addrs.AbsProviderInstanceCorrect

	// ResourceType is the resource type of the object this plan is for, as
	// would be understood by the provider that generated this plan.
	ResourceType string

	// RequiredResourceInstances are the addresses of zero or more resource
	// instances that must exist and must be fully converged before the
	// final plan for this resource instance could be calculated.
	//
	// These addresses can potentially contain unknown instance keys if the
	// configuration for this resource instance was derived from placeholders
	// for upstream resource instances that had unknown keys in their own
	// addresses.
	RequiredResourceInstances addrs.Set[addrs.AbsResourceInstance]

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

	// CreateProvisioners are the provisioners to execute if the resource
	// instance is being created.
	CreateProvisioners []eval.Provisioner
	// DestroyProvisioners are the provisioners to execute if the resource
	// instance is being destroyed.
	DestroyProvisioners []eval.Provisioner
}

// IntoDeposed returns a new [ManagedResourceObjectFinalPlan] that represents
// the same change as the receiver but has DeposedKey set the given value.
//
// Note that the result is only a shallow copy of the reciever, so nothing
// reachable through pointers should be modified in either object. Final plan
// objects are immutable by convention.
//
// This function does not (and cannot) verify that the chosen deposed key is
// unique for the resource instance. It's the caller's responsibility to
// allocate a unique deposed key to use.
func (p *ManagedResourceObjectFinalPlan) IntoDeposed(key states.DeposedKey) *ManagedResourceObjectFinalPlan {
	ret := *p // shallow copy
	ret.Addr.DeposedKey = key
	return &ret
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
	Addr addrs.AbsResourceInstanceObject

	// State is the object currently associated with the given address.
	State *states.ResourceInstanceObjectFull
}

// IntoCurrent returns a new [ResourceInstanceObject] that has the same
// State as the receiver but has DeposedKey set to [states.NotDeposed].
func (o *ResourceInstanceObject) IntoCurrent() *ResourceInstanceObject {
	return &ResourceInstanceObject{
		Addr:  o.Addr.InstanceAddr.CurrentObject(),
		State: o.State,
	}
}

// IntoDeposed returns a new [ResourceInstanceObject] that has the same
// State as the receiver but has DeposedKey set the given value.
//
// This function does not (and cannot) verify that the chosen deposed key is
// unique for the resource instance. It's the caller's responsibility to
// allocate a unique deposed key to use.
func (o *ResourceInstanceObject) IntoDeposed(key states.DeposedKey) *ResourceInstanceObject {
	if key == states.NotDeposed {
		panic("ResourceInstanceObject.IntoDeposed called without a deposed key")
	}
	return &ResourceInstanceObject{
		Addr:  o.Addr.InstanceAddr.Object(key),
		State: o.State,
	}
}

// WithNewAddr returns a new [ResourceInstanceObject] that has the same
// State as the receiver but has InstanceAddr set to the given address.
func (o *ResourceInstanceObject) WithNewAddr(addr addrs.AbsResourceInstance) *ResourceInstanceObject {
	return &ResourceInstanceObject{
		Addr:  addr.Object(o.Addr.DeposedKey),
		State: o.State,
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
		Addr:  o.Addr,
		State: newState,
	}
}

// ResourceInstanceObjectMeta represents various metadata for a resource
// instance object.
//
// "Metadata" is loosely defined as including the sort of information we rely
// on even when an object is no longer "desired" and thus we'd plan to delete
// it, and so there's no normal declaration for the resource instance left
// in the configuration anymore but nonetheless there might be other information
// assembled from any combination of the following:
//   - Metadata that was copied from configuration into prior state in the
//     previous round.
//   - Arguments in a "removed" block that's acting as a sort of "tombstone" for
//     a previously-present resource block.
//   - The resource block that an "orphan" resource instance was previously
//     declared from, which remains in the configuration and possibly declares
//     some settings that apply to all instances of the resource.
//
// This type lives at the execution engine layer because it's an abstraction
// over a mixture of information from the configuration and information from
// the prior state, whereas the "evaluator"'s scope is strictly limited only
// to the configuration.
type ResourceInstanceObjectMeta struct {
	// Addr identifies which resource instance object this metadata applies to.
	//
	// When an existing object will be moved to a new address during the
	// apply phase, for example using a "moved" block, this reflects the address
	// it's expected to have at the end of a successful apply phase. The address
	// in this field must therefore NOT be used to identify objects to retrieve
	// from the prior state.
	//
	// However, for objects being replaced in the create-then-destroy order note
	// that successful execution causes the original object to be deleted and a
	// new one to be created at the same address, and in that case we use the
	// address where the new object would be placed instead of the address
	// that the old object would be temporarily deposed to during the process.
	// This reflects a small inconsistency/ambiguity in our usual terminology
	// where "object" normally refers to the actual remote object, but in this
	// case it refers only to the _object address_ from OpenTofu's perspective,
	// and two different remote objects will appear at this address over
	// the course of the apply phase.
	Addr addrs.AbsResourceInstanceObject

	// ProviderInstance is the address of the provider instance that is
	// currently considered responsible for this resource instance object.
	//
	// A resource instance object is associated with a specific provider
	// instance throughout a plan/apply round, but may change which provider
	// instance it is associated with between rounds based on changes in the
	// configuration.
	ProviderInstance exprs.FromValue[addrs.AbsProviderInstanceCorrect]

	// ResourceType is the resource type of the object this metadata is for,
	// as would be understood by the provider identified in
	// [ResourceInstanceObjectMeta.ProviderInstance].
	ResourceType string

	// PostCreateProvisioners are the provisioners to execute immediately after
	// the resource instance object has been created.
	//
	// The contents of this field can only be relied on during a round where
	// the apply phase would create a resource instance object at the associated
	// address. Its contents are unspecified in other cases.
	PostCreateProvisioners []eval.ResourceProvisioner

	// PreDeleteProvisioners are the provisioners to execute immediately before
	// the resource instance object will be deleted.
	//
	// The contents of this field can only be relied on during a round where
	// the apply phase would delete a resource instance object at the associated
	// address. Its contents are unspecified in other cases.
	PreDeleteProvisioners []eval.ResourceProvisioner
}

// BuildResourceInstanceObjectMeta constructs a [ResourceInstanceObjectMeta]
// object that incorporates information from both the configuration and the
// prior state, generally preferring to use the configuration information when
// possible but using the prior state as a fallback.
//
// This is our primary logic for deciding the effective metadata for a resource
// instance object based on all of the information currently known. The planning
// and applying engines should both use this function to ensure that they always
// agree about the metadata for a given resource instance object.
//
// It's the caller's responsibility to ensure that all of the arguments agree
// about which resource instance object they are describing. The given object
// address will be the value of [ResourceInstanceObjectMeta.Addr] and so must
// be consistent with the documentation of that field.
//
// At least one of fromConfig and state must be non-nil, or this function will
// panic. There is no reason to ask for metadata for an object that exists in
// neither the desired nor the prior state.
func BuildResourceInstanceObjectMeta(
	addr addrs.AbsResourceInstanceObject,
	fromConfig *eval.ConfiguredResourceInstanceObjectMeta,
	state *states.ResourceInstanceObjectFull,
) *ResourceInstanceObjectMeta {
	if fromConfig == nil && state == nil {
		panic(fmt.Sprintf("cannot build resource instance object metadata for %s with neither configured nor prior state metadata", addr))
	}
	ret := &ResourceInstanceObjectMeta{
		Addr: addr,
	}

	// If both state and fromConfig are present then we'll start with state
	// so that the config values can potentially override.
	if state != nil {
		// Note that if this object is participating in a cross-resource-type
		// move in this plan/apply round this will initially reflect the
		// old resource type, but then we'll overwrite it with the new resource
		// type from the configuration object below.
		ret.ResourceType = state.ResourceType
	}

	// TODO: The rest of this

	return ret
}
