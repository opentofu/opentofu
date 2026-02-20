// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/plugins"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type contextPlugins struct {
	providers    plugins.ProviderManager
	provisioners plugins.ProvisionerManager
}

func newContextPlugins(library plugins.Library) *contextPlugins {
	if library == nil {
		// We are in a *_test.go that should be fixed
		library = plugins.NewLibrary(nil, nil)
	}
	return &contextPlugins{
		providers:    library.NewProviderManager(),
		provisioners: library.NewProvisionerManager(),
	}
}

func (cp *contextPlugins) HasProvider(addr addrs.Provider) bool {
	return cp.providers.HasProvider(addr)
}

func (cp *contextPlugins) HasProvisioner(typ string) bool {
	return cp.provisioners.HasProvisioner(typ)
}

// ProviderConfigSchema is a helper wrapper around ProviderSchema which first
// reads the full schema of the given provider and then extracts just the
// provider's configuration schema, which defines what's expected in a
// "provider" block in the configuration when configuring this provider.
func (cp *contextPlugins) ProviderConfigSchema(ctx context.Context, providerAddr addrs.Provider) (*configschema.Block, tfdiags.Diagnostics) {
	providerSchema, diags := cp.providers.GetProviderSchema(ctx, providerAddr)
	if diags.HasErrors() {
		return nil, diags
	}

	return providerSchema.Provider.Block, diags
}

// ResourceTypeSchema is a helper wrapper around ProviderSchema which first
// reads the schema of the given provider and then tries to find the schema
// for the resource type of the given resource mode in that provider.
//
// ResourceTypeSchema will return an error if the provider schema lookup
// fails, but will return nil if the provider schema lookup succeeds but then
// the provider doesn't have a resource of the requested type.
//
// Managed resource types have versioned schemas, so the second return value
// is the current schema version number for the requested resource. The version
// is irrelevant for other resource modes.
func (cp *contextPlugins) ResourceTypeSchema(ctx context.Context, providerAddr addrs.Provider, resourceMode addrs.ResourceMode, resourceType string) (*configschema.Block, uint64, tfdiags.Diagnostics) {
	providerSchema, diags := cp.providers.GetProviderSchema(ctx, providerAddr)
	if diags.HasErrors() {
		return nil, 0, diags
	}

	schema, version := providerSchema.SchemaForResourceType(resourceMode, resourceType)
	return schema, version, diags
}

// ProvisionerSchema uses a temporary instance of the provisioner with the
// given type name to obtain the schema for that provisioner's configuration.
//
// ProvisionerSchema memoizes results by provisioner type name, so it's fine
// to repeatedly call this method with the same name if various different
// parts of OpenTofu all need the same schema information.
func (cp *contextPlugins) ProvisionerSchema(addr string) (*configschema.Block, error) {
	return cp.provisioners.ProvisionerSchema(addr)
}
