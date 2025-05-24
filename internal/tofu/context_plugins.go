// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// contextPlugins represents a library of available plugins (providers and
// provisioners) which we assume will all be used with the same
// tofu.Context, and thus it'll be safe to cache certain information
// about the providers for performance reasons.
type contextPlugins struct {
	providerFactories    map[addrs.Provider]providers.Factory
	provisionerFactories map[string]provisioners.Factory

	// In cache we retain results from certain operations that we expect
	// should be constants for a particular version of a plugin, such as
	// a provider's schema, so that we can avoid the cost of re-fetching the
	// same data.
	cache contextPluginsCache
}

func newContextPlugins(providerFactories map[addrs.Provider]providers.Factory, provisionerFactories map[string]provisioners.Factory) *contextPlugins {
	return &contextPlugins{
		providerFactories:    providerFactories,
		provisionerFactories: provisionerFactories,
	}
}

func (cp *contextPlugins) HasProvider(addr addrs.Provider) bool {
	_, ok := cp.providerFactories[addr]
	return ok
}

func (cp *contextPlugins) NewProviderInstance(addr addrs.Provider) (providers.Interface, error) {
	f, ok := cp.providerFactories[addr]
	if !ok {
		return nil, fmt.Errorf("unavailable provider %q", addr.String())
	}

	return f()

}

func (cp *contextPlugins) HasProvisioner(typ string) bool {
	_, ok := cp.provisionerFactories[typ]
	return ok
}

func (cp *contextPlugins) NewProvisionerInstance(typ string) (provisioners.Interface, error) {
	f, ok := cp.provisionerFactories[typ]
	if !ok {
		return nil, fmt.Errorf("unavailable provisioner %q", typ)
	}

	return f()
}

// LoadProviderSchemas starts a background task to load the schemas for any
// providers used by the given configuration and state, either of which may
// be nil to represent their absence.
//
// This function returns immediately but subsequent calls to access provider
// schemas will then block until the background work has completed, so it's
// better to call this function as early as possible and then delay accessing
// provider schema information for as long as possible after that to achieve
// the biggest concurrency benefit.
func (cp *contextPlugins) LoadProviderSchemas(ctx context.Context, config *configs.Config, state *states.State) {
	cp.cache.LoadProviderSchemas(ctx, config, state, cp.providerFactories)
}

// ProviderSchema returns the schema information for the given provider
// from a cache previously populated by call to
// [contextPlugins.LoadProviderSchemas].
//
// If the background work started by an earlier
// [contextPlugins.LoadProviderSchemas] is still in progress then this function
// blocks until that work is complete. However, this function never makes any
// provider calls directly itself.
//
// If the requested provider was not included in a previous call to
// [contextPlugins.LoadProviderSchemas] then this returns diagnostics.
func (cp *contextPlugins) ProviderSchema(addr addrs.Provider) (providers.ProviderSchema, tfdiags.Diagnostics) {
	resp := cp.cache.GetProviderSchemaResponse(addr)

	// The underlying provider API includes diagnostics inline in the response
	// due to quirks of the mapping to gRPC, but we'll adapt that here to be
	// more like how we conventionally treat diagnostics so that our caller
	// can follow the usual diagnostics-handling patterns.
	//
	// GetProviderSchemaResponse is guaranteed to always return a non-nil
	// result, since it'll synthesize an error response itself if there is
	// not already a cached entry for this provider.
	return *resp, resp.Diagnostics
}

// ProviderConfigSchema is a helper wrapper around ProviderSchema which first
// retrieves the full schema of the given provider and then extracts just the
// provider's configuration schema, which defines what's expected in a
// "provider" block in the configuration when configuring this provider.
func (cp *contextPlugins) ProviderConfigSchema(providerAddr addrs.Provider) (*configschema.Block, tfdiags.Diagnostics) {
	providerSchema, diags := cp.ProviderSchema(providerAddr)
	if diags.HasErrors() {
		return nil, diags
	}
	return providerSchema.Provider.Block, diags
}

// ResourceTypeSchema is a helper wrapper around ProviderSchema which first
// retrieves the schema of the given provider and then tries to find the schema
// for the resource type of the given resource mode in that provider.
//
// ResourceTypeSchema will return an error if the provider schema lookup
// fails, but will return nil if the provider schema lookup succeeds but then
// the provider doesn't have a resource of the requested type.
//
// Managed resource types have versioned schemas, so the second return value
// is the current schema version number for the requested resource. The version
// is irrelevant for other resource modes.
func (cp *contextPlugins) ResourceTypeSchema(providerAddr addrs.Provider, resourceMode addrs.ResourceMode, resourceType string) (*configschema.Block, uint64, tfdiags.Diagnostics) {
	providerSchema, diags := cp.ProviderSchema(providerAddr)
	if diags.HasErrors() {
		return nil, 0, diags
	}

	schema, version := providerSchema.SchemaForResourceType(resourceMode, resourceType)
	return schema, version, nil
}

// ProvisionerSchema uses a temporary instance of the provisioner with the
// given type name to obtain the schema for that provisioner's configuration.
//
// Provisioner schemas are currently not cached because we assume that it's
// rare to use any except those compiled directly into OpenTofu, and therefore
// we're usually just retrieving an already-resident data structure from a
// different part of the program. This could potentially be slow for those
// using the legacy support for plugin-based provisioners, if they have many
// instances of such provisioners.
func (cp *contextPlugins) ProvisionerSchema(typ string) (*configschema.Block, error) {
	log.Printf("[TRACE] tofu.contextPlugins: Initializing provisioner %q to read its schema", typ)
	provisioner, err := cp.NewProvisionerInstance(typ)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate provisioner %q to obtain schema: %w", typ, err)
	}
	defer provisioner.Close()

	resp := provisioner.GetSchema()
	if resp.Diagnostics.HasErrors() {
		return nil, fmt.Errorf("failed to retrieve schema from provisioner %q: %w", typ, resp.Diagnostics.Err())
	}

	return resp.Provisioner, nil
}
