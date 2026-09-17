// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ManagedResourceType represents a named resource type in a specific provider,
// and also carries a client for interacting with that provider.
//
// Most methods of this type relate to managed-resource-related operations in
// the underlying provider protocol, but also include additional OpenTofu-level
// logic such as verifying that the provider is correctly implementing the
// protocol's constraints on how objects are allowed to change.
type ManagedResourceType struct {
	// providerAddr is the provider that this resource type belongs to.
	providerAddr addrs.Provider

	// typeName is the resource type name as expected by the associated provider.
	typeName string

	// client is the client to use to interact with the provider that this
	// resource type belongs to.
	client providers.Interface
}

var _ ResourceType = (*ManagedResourceType)(nil)

// NewManagedResourceType constructs a new [ManagedResourceType] for the
// given resource type name in the provider whose client is provided.
//
// It's the caller's responsibility to make sure that the given client is
// actually for the provider indicated.
func NewManagedResourceType(providerAddr addrs.Provider, typeName string, client providers.Interface) *ManagedResourceType {
	return &ManagedResourceType{
		providerAddr: providerAddr,
		typeName:     typeName,
		client:       client,
	}
}

// ResourceMode implements [ResourceType].
func (rt *ManagedResourceType) ResourceMode() addrs.ResourceMode {
	return addrs.ManagedResourceMode
}

// ResourceTypeName implements [ResourceType].
func (rt *ManagedResourceType) ResourceTypeName() string {
	return rt.typeName
}

// LoadSchema loads the schema for this resource type from its provider.
//
// This method performs no direct caching of the result, so the underlying
// provider client (originally passed to [NewManagedResourceType]) should
// provide its own caching.
func (rt *ManagedResourceType) LoadSchema(ctx context.Context) (providers.Schema, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// This is awkward because we already have higher-level objects that
	// can answer this question given an entire provider manager, but by the
	// time we get here we've already used the provider manager to instantiate
	// the client and no longer have access to the manager.
	//
	// TODO: Find a different way to structure this so that this concern
	// can be centralized in one place while still accessing it indirectly
	// through a fully-encapsulated "resource type" object that we can pass
	// around independently of the plugin library it came from. The overall
	// idea here is to move away from the pattern of passing around
	// the provider manager, provider address, and resource type name as
	// three separate arguments to functions and have it all encapsulated
	// in a single object we can pass around, similar to how exprs.Valuer
	// encapsulates everything needed to evaluate something.

	resp := rt.client.GetProviderSchema(ctx)
	diags = diags.Append(resp.Diagnostics)
	if resp.Diagnostics.HasErrors() {
		return providers.Schema{}, diags
	}
	ret, ok := resp.ResourceTypes[rt.typeName]
	if !ok {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Unsupported resource type",
			fmt.Sprintf("Provider %s does not support a managed resource type named %q.", rt.providerAddr.String(), rt.typeName),
		))
		return providers.Schema{}, diags
	}

	return ret, diags
}

// ReplaceOrder represents potential restrictions on which order the "create"
// and "delete" actions must be taken when replacing a managed resource
// instance object.
type ReplaceOrder byte

//go:generate go tool golang.org/x/tools/cmd/stringer -type ReplaceOrder

const (
	// ReplaceAnyOrder means that there is no constraint on which order the
	// "create" and "delete" operations must happen in, and so the order can
	// be selected to suit the constraints of other resource instace objects
	// in a dependency chain.
	//
	// This particular [ReplaceOrder] value is valid only early in the planning
	// phase when each resource instance object is reporting just its own
	// requirements. Part of the work of the planning phase is to decide on a
	// specific order to use for each resource instance object, and so after
	// that step this value is no longer suitable.
	ReplaceAnyOrder ReplaceOrder = iota

	// ReplaceCreateFirst means that a new object must be created before the
	// previous object is deleted.
	//
	// This implies that temporarily there are two objects, with the previous
	// object considered to be "deposed" during the overlap. This isn't possible
	// for all resource instances due to uniqueness constraints in the remote
	// system.
	ReplaceCreateFirst

	// ReplaceDeleteFirst means that the previous object must be deleted before
	// the new object is created.
	//
	// This implies that there will temporarily be no objects at all, but this
	// is sometimes necessary because of uniqueness constraints in the remote
	// system.
	ReplaceDeleteFirst
)
