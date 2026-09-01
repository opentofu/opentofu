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
	"github.com/opentofu/opentofu/internal/plugins"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Schemas is the original name for [plugins.Schemas], preserved as an alias
// here for now to preserve existing references to this type.
//
// Use [plugins.Schemas] directly in new code.
type Schemas = plugins.Schemas

// loadSchemas searches the given configuration, state  and plan (any of which
// may be nil) for constructs that have an associated schema, requests the
// necessary schemas from the given component factory (which must _not_ be nil),
// and returns a single object representing all the necessary schemas.
//
// If an error is returned, it may be a wrapped tfdiags.Diagnostics describing
// errors across multiple separate objects. Errors here will usually indicate
// either misbehavior on the part of one of the providers or of the provider
// protocol itself. When returned with errors, the returned schemas object is
// still valid but may be incomplete.
func loadSchemas(ctx context.Context, config *configs.Config, state *states.State, plugins *contextPlugins) (*plugins.Schemas, error) {
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
			schema, schemaDiags := plugins.providers.GetProviderSchema(ctx, fqn)

			// Ensure that we don't race on diags or schemas now that the hard work is done
			lock.Lock()
			defer lock.Unlock()

			if schemaDiags.HasErrors() {
				diags = diags.Append(schemaDiags)
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
