// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package evalglue

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
)

// ConfiguredResourceInstanceObjectMeta is the true internal name of what
// external packages know as [eval.ConfiguredResourceInstanceObjectMeta],
// defined here to avoid import cycles when this is used by language edition
// and configgraph code.
type ConfiguredResourceInstanceObjectMeta struct {
	// Provider, ResourceMode, and ResourceType together identify a specific
	// resource type in the terms expected by the provider.
	//
	// In particular the resource type given here is the one to send in requests
	// to the identified provider, and not for use elsewhere. Use
	// [addrs.Resource.Type] (from an address value describing the same object)
	// as the resource type internally within the language runtime and execution
	// engines.
	Provider     addrs.Provider
	ResourceMode addrs.ResourceMode
	ResourceType string

	// ProviderInstance is the address of the specific provider instance that
	// this object is currently configured to belong to.
	//
	// This value is unknown if the configured selection is derived from
	// an unknown value, or nil if there is no configured selection at all. In
	// the absence of a configured selection, callers should probably try to
	// fall back to a selection from the prior state instead.
	ProviderInstance exprs.FromValue[*addrs.AbsProviderInstanceCorrect]

	// CreateBeforeDelete is true if this object is configured to force creating
	// a new remote object before destroying the current one when performing
	// a "replace" action. Otherwise either ordering is allowed and delete
	// happens first by default unless the other ordering is forced by a
	// dependent object having this set to true.
	//
	// This setting also affects how actions for this object may be ordered
	// with actions from other objects even when not replacing, in order to
	// produce a well-defined execution order when this object's actions are
	// combined with actions of other objects in the apply-time execution graph.
	//
	// This field is relevant only for managed resource mode and its value is
	// unspecified for other resource modes.
	CreateBeforeDelete exprs.FromValue[bool]

	// DeleteWhenRemoved is true if the expected treatment for a non-desired
	// object at this address is to ask the associated provider to delete it,
	// or false if the expected treatment is just to "forget" it by removing
	// the binding from the state without notifying the provider.
	//
	// This field is relevant only for managed resource mode and its value is
	// unspecified for other resource modes.
	DeleteWhenRemoved exprs.FromValue[bool]

	// DeletionInvalid is true if the author has configured that any execution
	// plan that involves deleting this object should be considered immediately
	// invalid. If false then deleting is a valid action to include.
	//
	// This field is relevant only for managed resource mode and its value is
	// unspecified for other resource modes.
	DeletionInvalid exprs.FromValue[bool]

	// TODO: Some representation of "ignore_changes", which the planning engine
	// will use as part of deciding which action to take.

	// TODO: Some representation of "replace_triggered_by", which the planning
	// engine will use to force a "replace" action where an "update" might
	// otherwise have been sufficient.

	// PostCreateProvisioners and PreDestroyProvisioners both represent a
	// sequence of provisioners configured for this resource instance object.
	//
	// These fields are relevant only for managed resource mode and their
	// content is unspecified for other resource modes.
	PostCreateProvisioners, PreDestroyProvisioners []*ResourceProvisioner
}

// ResourceProvisioner represents a single provisioner configured for a
// resource instance object.
type ResourceProvisioner struct {
	// Type and Config represent the type of provisioner to run (e.g "local-exec")
	// and a configuration object suitable for that provisioner type.
	Type   string
	Config cty.Value

	// ConnectionConfig is an object representation of the configuration for how
	// to connect to a remote system to run the provisioner.
	//
	// Not all provisioner types make remote connections. Those that don't need
	// it will just ignore this field completely.
	ConnectionConfig cty.Value
}
