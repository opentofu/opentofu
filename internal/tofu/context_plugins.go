// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/zclconf/go-cty/cty/function"
)

// contextPlugins represents a library of available plugins (providers and
// provisioners) which we assume will all be used with the same
// tofu.Context, and thus it'll be safe to cache certain information
// about the providers for performance reasons.
type contextPlugins struct {
	providerFactories    map[addrs.Provider]providers.Factory
	providerFunctions    map[addrs.Provider]map[string]function.Function
	provisionerFactories map[string]provisioners.Factory
}

func newContextPlugins(providerFactories map[addrs.Provider]providers.Factory, provisionerFactories map[string]provisioners.Factory) (*contextPlugins, error) {
	ret := &contextPlugins{
		providerFactories:    providerFactories,
		provisionerFactories: provisionerFactories,
	}

	// This is a bit convoluted as we need to use the ProviderSchema function call below to
	// validate and initialize the provider schemas.  Long term the whole provider abstraction
	// needs to be re-thought.
	var err error
	ret.providerFunctions, err = ret.buildProviderFunctions()
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// Loop through all of the providerFactories and build a map of addr -> functions
// As a side effect, this initialzes the schema cache if not already initialized, with the proper validation path.
func (cp *contextPlugins) buildProviderFunctions() (map[addrs.Provider]map[string]function.Function, error) {
	funcs := make(map[addrs.Provider]map[string]function.Function)

	// Pull all functions out of given providers
	for addr, factory := range cp.providerFactories {
		addr := addr
		factory := factory

		// Before functions, the provider schemas were already pre-loaded and cached.  That initial caching
		// has been moved here.  When the provider abstraction layers are refactored, this could instead
		// expose and use provider.GetFunctions instead of needing to load and cache the whole schema.
		// However, at the time of writing there is no benefit to defer caching these schemas in code
		// paths which build a tofu.Context.
		schema, err := cp.ProviderSchema(addr)
		if err != nil {
			return nil, err
		}

		funcs[addr] = providerFunctions(addr, schema.Functions, factory)
	}
	return funcs, nil
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

// ProviderSchema uses a temporary instance of the provider with the given
// address to obtain the full schema for all aspects of that provider.
//
// ProviderSchema memoizes results by unique provider address, so it's fine
// to repeatedly call this method with the same address if various different
// parts of OpenTofu all need the same schema information.
func (cp *contextPlugins) ProviderSchema(addr addrs.Provider) (providers.ProviderSchema, error) {
	// Check the global schema cache first.
	// This cache is only written by the provider client, and transparently
	// used by GetProviderSchema, but we check it here because at this point we
	// may be able to avoid spinning up the provider instance at all.
	//
	// It's worth noting that ServerCapabilities.GetProviderSchemaOptional is ignored here.
	// That is because we're checking *prior* to the provider's instantiation.
	// GetProviderSchemaOptional only says that *if we instantiate a provider*,
	// then we need to run the get schema call at least once.
	// BUG This SHORT CIRCUITS the logic below and is not the only code which inserts provider schemas into the cache!!
	schemas, ok := providers.SchemaCache.Get(addr)
	if ok {
		log.Printf("[TRACE] tofu.contextPlugins: Serving provider %q schema from global schema cache", addr)
		return schemas, nil
	}

	log.Printf("[TRACE] tofu.contextPlugins: Initializing provider %q to read its schema", addr)
	provider, err := cp.NewProviderInstance(addr)
	if err != nil {
		return schemas, fmt.Errorf("failed to instantiate provider %q to obtain schema: %w", addr, err)
	}
	defer provider.Close()

	resp := provider.GetProviderSchema()
	if resp.Diagnostics.HasErrors() {
		return resp, fmt.Errorf("failed to retrieve schema from provider %q: %w", addr, resp.Diagnostics.Err())
	}

	if resp.Provider.Version < 0 {
		// We're not using the version numbers here yet, but we'll check
		// for validity anyway in case we start using them in future.
		return resp, fmt.Errorf("provider %s has invalid negative schema version for its configuration blocks,which is a bug in the provider ", addr)
	}

	for t, r := range resp.ResourceTypes {
		if err := r.Block.InternalValidate(); err != nil {
			return resp, fmt.Errorf("provider %s has invalid schema for managed resource type %q, which is a bug in the provider: %w", addr, t, err)
		}
		if r.Version < 0 {
			return resp, fmt.Errorf("provider %s has invalid negative schema version for managed resource type %q, which is a bug in the provider", addr, t)
		}
	}

	for t, d := range resp.DataSources {
		if err := d.Block.InternalValidate(); err != nil {
			return resp, fmt.Errorf("provider %s has invalid schema for data resource type %q, which is a bug in the provider: %w", addr, t, err)
		}
		if d.Version < 0 {
			// We're not using the version numbers here yet, but we'll check
			// for validity anyway in case we start using them in future.
			return resp, fmt.Errorf("provider %s has invalid negative schema version for data resource type %q, which is a bug in the provider", addr, t)
		}
	}

	return resp, nil
}

// ProviderConfigSchema is a helper wrapper around ProviderSchema which first
// reads the full schema of the given provider and then extracts just the
// provider's configuration schema, which defines what's expected in a
// "provider" block in the configuration when configuring this provider.
func (cp *contextPlugins) ProviderConfigSchema(providerAddr addrs.Provider) (*configschema.Block, error) {
	providerSchema, err := cp.ProviderSchema(providerAddr)
	if err != nil {
		return nil, err
	}

	return providerSchema.Provider.Block, nil
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
func (cp *contextPlugins) ResourceTypeSchema(providerAddr addrs.Provider, resourceMode addrs.ResourceMode, resourceType string) (*configschema.Block, uint64, error) {
	providerSchema, err := cp.ProviderSchema(providerAddr)
	if err != nil {
		return nil, 0, err
	}

	schema, version := providerSchema.SchemaForResourceType(resourceMode, resourceType)
	return schema, version, nil
}

// ProvisionerSchema uses a temporary instance of the provisioner with the
// given type name to obtain the schema for that provisioner's configuration.
//
// ProvisionerSchema memoizes results by provisioner type name, so it's fine
// to repeatedly call this method with the same name if various different
// parts of OpenTofu all need the same schema information.
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

// Functions provides a map of provider::alias:function for a given provider type.
// All providers of a given type us the same functions and provider instance and
// additional aliases do not incur any performance penalty.
func (cp *contextPlugins) Functions(addr addrs.Provider, alias string) map[string]function.Function {
	funcs := cp.providerFunctions[addr]
	aliased := make(map[string]function.Function)
	for name, fn := range funcs {
		aliased[fmt.Sprintf("provider::%s::%s", alias, name)] = fn
	}
	return aliased
}
