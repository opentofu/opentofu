// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Schemas is a container for various kinds of schema that OpenTofu needs
// during processing.
type Schemas struct {
	Providers    map[addrs.Provider]providers.ProviderSchema
	Provisioners map[string]*configschema.Block
}

// ProviderSchema returns the entire ProviderSchema object that was produced
// by the plugin for the given provider, or nil if no such schema is available.
//
// It's usually better to go use the more precise methods offered by type
// Schemas to handle this detail automatically.
func (ss *Schemas) ProviderSchema(provider addrs.Provider) providers.ProviderSchema {
	return ss.Providers[provider]
}

// ProviderConfig returns the schema for the provider configuration of the
// given provider type, or nil if no such schema is available.
func (ss *Schemas) ProviderConfig(provider addrs.Provider) *configschema.Block {
	return ss.ProviderSchema(provider).Provider.Block
}

// ResourceTypeConfig returns the schema for the configuration of a given
// resource type belonging to a given provider type, or nil of no such
// schema is available.
//
// In many cases the provider type is inferable from the resource type name,
// but this is not always true because users can override the provider for
// a resource using the "provider" meta-argument. Therefore it's important to
// always pass the correct provider name, even though it many cases it feels
// redundant.
func (ss *Schemas) ResourceTypeConfig(provider addrs.Provider, resourceMode addrs.ResourceMode, resourceType string) (block *configschema.Block, schemaVersion uint64) {
	ps := ss.ProviderSchema(provider)
	return ps.SchemaForResourceType(resourceMode, resourceType)
}

// ProvisionerConfig returns the schema for the configuration of a given
// provisioner, or nil of no such schema is available.
func (ss *Schemas) ProvisionerConfig(name string) *configschema.Block {
	return ss.Provisioners[name]
}

// loadSchemas searches the given configuration, state  and plan (any of which
// may be nil) for constructs that have an associated schema, requests the
// necessary schemas from the given component factory (which must _not_ be nil),
// and returns a single object representing all of the necessary schemas.
//
// If an error is returned, it may be a wrapped tfdiags.Diagnostics describing
// errors across multiple separate objects. Errors here will usually indicate
// either misbehavior on the part of one of the providers or of the provider
// protocol itself. When returned with errors, the returned schemas object is
// still valid but may be incomplete.
func loadSchemas(ctx context.Context, config *configs.Config, state *states.State, plugins *contextPlugins) (*Schemas, error) {
	var diags tfdiags.Diagnostics

	provisioners, provisionerDiags := loadProvisionerSchemas(ctx, config, plugins)
	diags = diags.Append(provisionerDiags)

	providers, providerDiags := loadProviderSchemas(ctx, config, state, plugins)
	diags = diags.Append(providerDiags)

	return &Schemas{
		Providers:    providers,
		Provisioners: provisioners,
	}, diags.Err()
}

func loadProviderSchemas(ctx context.Context, config *configs.Config, state *states.State, plugins *contextPlugins) (map[addrs.Provider]providers.ProviderSchema, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	schemas := map[addrs.Provider]providers.ProviderSchema{}

	if config != nil {
		for _, fqn := range config.ProviderTypes() {
			schemas[fqn] = providers.ProviderSchema{}
		}
	}

	if state != nil {
		needed := providers.AddressedTypesAbs(state.ProviderAddrs())
		for _, fqn := range needed {
			schemas[fqn] = providers.ProviderSchema{}
		}
	}

	var wg sync.WaitGroup
	var lock sync.Mutex
	lock.Lock() // Prevent anything from started until we have finished schema map reads
	for fqn := range schemas {
		wg.Go(func() {
			log.Printf("[TRACE] LoadSchemas: retrieving schema for provider type %q", fqn.String())
			schema, err := plugins.ProviderSchema(ctx, fqn)

			// Ensure that we don't race on diags or schemas now that the hard work is done
			lock.Lock()
			defer lock.Unlock()

			if err != nil {
				diags = diags.Append(err)
				return
			}

			schemas[fqn] = schema
		})
	}

	// Allow execution to start now that reading of schemas map has completed
	lock.Unlock()

	// Wait for all of the scheduled routines to complete
	wg.Wait()

	return schemas, diags
}

func loadProvisionerSchemas(ctx context.Context, config *configs.Config, plugins *contextPlugins) (map[string]*configschema.Block, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	schemas := map[string]*configschema.Block{}

	// Determine the full list of provisioners recursively
	var addProvisionersToSchema func(config *configs.Config)
	addProvisionersToSchema = func(config *configs.Config) {
		if config == nil {
			return
		}
		for _, rc := range config.Module.ManagedResources {
			for _, pc := range rc.Managed.Provisioners {
				schemas[pc.Type] = &configschema.Block{}
			}
		}

		// Must also visit our child modules, recursively.
		for _, cc := range config.Children {
			addProvisionersToSchema(cc)
		}
	}
	addProvisionersToSchema(config)

	// Populate the schema entries
	for name := range schemas {
		log.Printf("[TRACE] LoadSchemas: retrieving schema for provisioner %q", name)
		schema, err := plugins.ProvisionerSchema(name)
		if err != nil {
			// We'll put a stub in the map so we won't re-attempt this on
			// future calls, which would then repeat the same error message
			// multiple times.
			schemas[name] = &configschema.Block{}
			diags = diags.Append(
				tfdiags.Sourceless(
					tfdiags.Error,
					"Failed to obtain provisioner schema",
					fmt.Sprintf("Could not load the schema for provisioner %q: %s.", name, err),
				),
			)
			continue
		}

		schemas[name] = schema
	}

	return schemas, diags
}
