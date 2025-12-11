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
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
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
func loadSchemas(ctx context.Context, providerFactories map[addrs.Provider]providers.Factory, provisionerFactories map[string]provisioners.Factory) (*Schemas, error) {
	var diags tfdiags.Diagnostics

	provisioners, provisionerDiags := loadProvisionerSchemas(ctx, provisionerFactories)
	diags = diags.Append(provisionerDiags)

	providers, providerDiags := loadProviderSchemas(ctx, providerFactories)
	diags = diags.Append(providerDiags)

	return &Schemas{
		Providers:    providers,
		Provisioners: provisioners,
	}, diags.Err()
}

func loadProviderSchemas(ctx context.Context, providerFactories map[addrs.Provider]providers.Factory) (map[addrs.Provider]providers.ProviderSchema, tfdiags.Diagnostics) {
	var lock sync.Mutex

	schemas := map[addrs.Provider]providers.ProviderSchema{}
	var diags tfdiags.Diagnostics

	var wg sync.WaitGroup
	for fqn, factory := range providerFactories {
		wg.Go(func() {
			log.Printf("[TRACE] loadProviderSchemas: retrieving schema for provider type %q", fqn.String())

			// Heavy lifting
			schema, err := func() (providers.ProviderSchema, error) {
				log.Printf("[TRACE] loadProviderSchemas: Initializing provider %q to read its schema", fqn)
				provider, err := factory()
				if err != nil {
					return providers.ProviderSchema{}, fmt.Errorf("failed to instantiate provider %q to obtain schema: %w", fqn, err)
				}
				defer provider.Close(ctx)

				resp := providers.ProviderSchema(provider.GetProviderSchema(ctx))
				if resp.Diagnostics.HasErrors() {
					return resp, fmt.Errorf("failed to retrieve schema from provider %q: %w", fqn, resp.Diagnostics.Err())
				}

				if err := resp.Validate(fqn); err != nil {
					return resp, err
				}

				return resp, nil
			}()

			// Ensure that we don't race on diags or schemas now that the hard work is done
			lock.Lock()
			defer lock.Unlock()
			schemas[fqn] = schema
			diags = diags.Append(err)
		})
	}

	// Wait for all of the scheduled routines to complete
	wg.Wait()

	return schemas, diags
}

func loadProvisionerSchemas(ctx context.Context, provisioners map[string]provisioners.Factory) (map[string]*configschema.Block, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	schemas := map[string]*configschema.Block{}

	// Populate the schema entries
	for name, factory := range provisioners {
		log.Printf("[TRACE] loadProvisionerSchemas: retrieving schema for provisioner %q", name)

		schema, err := func() (*configschema.Block, error) {
			log.Printf("[TRACE] loadProvisionerSchemas: Initializing provisioner %q to read its schema", name)
			provisioner, err := factory()
			if err != nil {
				return nil, fmt.Errorf("failed to instantiate provisioner %q to obtain schema: %w", name, err)
			}
			defer provisioner.Close()

			resp := provisioner.GetSchema()
			if resp.Diagnostics.HasErrors() {
				return nil, fmt.Errorf("failed to retrieve schema from provisioner %q: %w", name, resp.Diagnostics.Err())
			}

			return resp.Provisioner, nil
		}()
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
