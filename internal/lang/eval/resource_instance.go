// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
)

// DesiredResourceInstance describes a resource instance that is part of
// the desired state (i.e. declared in the configuration).
//
// In situations where unknown values mean that we cannot yet determine exactly
// which instances exist for a given resource an object of this type can
// potentially instead represent a placeholder for zero or more instances
// of the same resource, in which case we can still produce a "speculative"
// planned new state but will not be able to actually apply changes to those
// resource instances until a subsequent plan/apply round.
// [DesiredResourceInstance.IsPlaceholder] returns true for these placeholder
// objects.
//
// Prior state resouce instances are not represented in this package at all.
// The plan and apply mechanisms implemented elsewhere are responsible for
// comparing desired resource instances with prior state resource instances
// to determine what actions are needed, if any.
type DesiredResourceInstance struct {
	// Addr is the absolute address of the resource instance, suitable for
	// correlation with resource instances in the prior state and (during the
	// apply phase) in the plan.
	//
	// For objects that represent placeholders for zero or more instances whose
	// instance expansion is not yet known,
	// [addrs.AbsResourceInstance.IsPlaceholder] returns true.
	//
	// Nothing about resource instance addresses should be exposed to providers
	// through the provider protocol, because exactly how we track resource
	// instances between rounds is a detail we want to be able to change later
	// without breaking existing providers. In particular, when populating a
	// resource type name in a request to a provider you should use the
	// ResourceType field of DesiredResourceInstance instead of fishing it out
	// from this address field.
	Addr addrs.AbsResourceInstance

	// ConfigVal is an object-typed value representing the configuration, which
	// has already been validated against the schema for the corresponding
	// resource type.
	//
	// This will contain unknown values if the configuration for this resource
	// instance is derived from the results of other resource instances which
	// have pending actions in this same plan.
	ConfigVal cty.Value

	// Provider is the source address of the provider that the resource type
	// of this resource instance belongs to.
	//
	// ProviderInstance is guaranteed to refer to an instance of this provider.
	Provider addrs.Provider

	// ProviderInstance is the absolute address of the provider instance that
	// this resource instance currently belongs to. All configured-provider
	// operations related to this resource instance must be performed through
	// this provider instance.
	//
	// This can be nil in situations where the decision about which provider
	// instance to use depends on an unknown value. In that case the planning
	// phase should return a canned placeholder object based only on the
	// configuration value and the schema for this resource type, such as
	// by using [objchange.ProposedNew], and should otherwise defer any
	// actions for this resource instance until a future plan/apply round.
	ProviderInstance *addrs.AbsProviderInstanceCorrect
	// ResourceMode and ResourceType are the resource type identifiers
	// as they would be understood by the provider specified in the Provider
	// and ProviderInstance fields.
	//
	// These is what should be sent to a provider plugin when making requests
	// to it. Today these always matches the similar values encoded in the
	// address given in the "Addr" field, but we're separating these so that
	// we're not duplicating that rule in many different parts of the system,
	// in case future change to OpenTofu cause the provider-facing
	// representation to differ from how it's exposed in the OpenTofu language.
	// (The representation in the provider protocol is much harder to change
	// because we want to stay backward-compatible with existing provider plugins.)
	ResourceMode addrs.ResourceMode
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

	// IgnoreChangesPaths are paths for which the module author requested
	// that we "ignore changes".
	//
	// To "ignore changes" means to disregard what is configured for anything
	// under a matching path in ConfigVal and to instead treat the corresponding
	// value from the prior state as the effective desired state. This is
	// meaningful only when planning in-place updates to an object that is
	// already tracked in the prior state; it should be ignored when planning
	// to create or delete the object associated with a resource instance.
	//
	// Index steps within the path can potentially have unknown keys if the
	// decision about what to ignore is based on a value that won't be known
	// until the apply phase.
	//
	// This is meaningful only for resource modes that support the "update"
	// change action, and so is always empty for other modes.
	IgnoreChangesPaths []cty.Path

	// CreateBeforeDestroy is true when the module author specified that
	// a "replace" action for this resource instance should be decomposed into
	// "create replacement and then destroy", instead of the default
	// decomposition of "destroy and then create replacement".
	//
	// How exactly that request is honored is outside the scope of this package,
	// and is instead the responsibility of the planning engine as it builds
	// the execution graph for the apply phase.
	//
	// This is meaningful only for resource modes that support the "update"
	// change action, and so is always false for other modes.
	//
	// FIXME: Probably also need an "unknown" representation for this, so
	// that we can eventually do https://github.com/opentofu/opentofu/issues/2523 .
	CreateBeforeDestroy bool

	// If RejectDeleteAction is true then the planning phase should return an
	// error if it would otherwise have planned to destroy any existing object
	// associated with this resource instance.
	//
	// This is meaningful only for resource modes that support the "update"
	// change action, and so is always false for other modes.
	//
	// FIXME: Probably also need an "unknown" representation for this, so
	// that we can eventually do https://github.com/opentofu/opentofu/issues/2522 .
	RejectDeleteAction bool

	// ReplaceTriggeredBy describes zero ore more attribute prefixes within
	// other resource instances for which the planning engine should force
	// replacement of this resource instance if any value beneath one of
	// the nominated paths has a change already planned for the current
	// plan/apply round.
	//
	// Index steps within the paths and instance keys within the resource
	// instance addresses can both potentially have unknown keys if the
	// decision about what to refer to is based on a value that won't be known
	// until the apply phase.
	//
	// This is meaningful only for resource modes that support the "update"
	// change action, and so is always false for other modes.
	//
	// Any resource instance mentioned in this collection will always also
	// appear in RequiredResourceInstances.
	ReplaceTriggeredBy []ResourceInstanceAttributePath
}

// IsPlaceholder returns true if this object is acting as a placeholder for
// zero or more resource instances whose full expansion is not yet known.
//
// In that case the other fields of [DesiredResourceInstance] describe
// characteristics that all of those instances would have in common, so that
// it's hopefully still possible to perform some speculative planning and
// transfer partial results downstream to other resource instances so we can
// present as complete as possible an overview of how the system is likely
// to be configured after subsequent plan/apply rounds once everything is fully
// converged.
//
// This is not the only reason why a particular resource instance might need
// to have its actions deferred to a future plan/apply round. Refer to the
// documentation for other fields of [DesiredResourceInstance] for some other
// situations where this is true. The planning engine might also have additional
// reasons for deferring particular resource instances that are outside the
// scope of the configuration.
func (ri *DesiredResourceInstance) IsPlaceholder() bool {
	return ri.Addr.IsPlaceholder()
}

// ResourceInstanceAttributePath describes a (possibly empty) attribute path
// within a resource instance.
type ResourceInstanceAttributePath struct {
	ResourceInstance addrs.AbsResourceInstance
	Path             cty.Path
}
