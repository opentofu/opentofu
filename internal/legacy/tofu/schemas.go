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
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Schemas is a container for various kinds of schema that OpenTofu needs
// during processing.
type Schemas struct {
	Providers    map[addrs.Provider]*ProviderSchema
	Provisioners map[string]*configschema.Block
}

// ProviderSchema returns the entire ProviderSchema object that was produced
// by the plugin for the given provider, or nil if no such schema is available.
//
// It's usually better to go use the more precise methods offered by type
// Schemas to handle this detail automatically.
func (ss *Schemas) ProviderSchema(provider addrs.Provider) *ProviderSchema {
	if ss.Providers == nil {
		return nil
	}
	return ss.Providers[provider]
}

// ProviderConfig returns the schema for the provider configuration of the
// given provider type, or nil if no such schema is available.
func (ss *Schemas) ProviderConfig(provider addrs.Provider) *configschema.Block {
	ps := ss.ProviderSchema(provider)
	if ps == nil {
		return nil
	}
	return ps.Provider
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
	if ps == nil || ps.ResourceTypes == nil {
		return nil, 0
	}
	return ps.SchemaForResourceType(resourceMode, resourceType)
}

// ProvisionerConfig returns the schema for the configuration of a given
// provisioner, or nil of no such schema is available.
func (ss *Schemas) ProvisionerConfig(name string) *configschema.Block {
	return ss.Provisioners[name]
}

// LoadSchemas searches the given configuration, state  and plan (any of which
// may be nil) for constructs that have an associated schema, requests the
// necessary schemas from the given component factory (which must _not_ be nil),
// and returns a single object representing all of the necessary schemas.
//
// If an error is returned, it may be a wrapped tfdiags.Diagnostics describing
// errors across multiple separate objects. Errors here will usually indicate
// either misbehavior on the part of one of the providers or of the provider
// protocol itself. When returned with errors, the returned schemas object is
// still valid but may be incomplete.
func LoadSchemas(config *configs.Config, state *states.State, components contextComponentFactory) (*Schemas, error) {
	schemas := &Schemas{
		Providers:    map[addrs.Provider]*ProviderSchema{},
		Provisioners: map[string]*configschema.Block{},
	}
	var diags tfdiags.Diagnostics

	newDiags := loadProviderSchemas(schemas.Providers, config, state, components)
	diags = diags.Append(newDiags)
	newDiags = loadProvisionerSchemas(schemas.Provisioners, config, components)
	diags = diags.Append(newDiags)

	return schemas, diags.Err()
}

func loadProviderSchemas(schemas map[addrs.Provider]*ProviderSchema, config *configs.Config, state *states.State, components contextComponentFactory) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	ensure := func(fqn addrs.Provider) {
		name := fqn.String()

		if _, exists := schemas[fqn]; exists {
			return
		}

		log.Printf("[TRACE] LoadSchemas: retrieving schema for provider type %q", name)
		provider, err := components.ResourceProvider(fqn)
		if err != nil {
			// We'll put a stub in the map so we won't re-attempt this on
			// future calls.
			schemas[fqn] = &ProviderSchema{}
			diags = diags.Append(
				fmt.Errorf("Failed to instantiate provider %q to obtain schema: %w", name, err),
			)
			return
		}
		defer func() {
			provider.Close(context.Background())
		}()

		resp := provider.GetProviderSchema(context.Background())
		if resp.Diagnostics.HasErrors() {
			// We'll put a stub in the map so we won't re-attempt this on
			// future calls.
			schemas[fqn] = &ProviderSchema{}
			diags = diags.Append(
				fmt.Errorf("Failed to retrieve schema from provider %q: %w", name, resp.Diagnostics.Err()),
			)
			return
		}

		s := &ProviderSchema{
			Provider:      resp.Provider.Block,
			ResourceTypes: make(map[string]*configschema.Block),
			DataSources:   make(map[string]*configschema.Block),

			ResourceTypeSchemaVersions: make(map[string]uint64),
		}

		if resp.Provider.Version < 0 {
			// We're not using the version numbers here yet, but we'll check
			// for validity anyway in case we start using them in future.
			diags = diags.Append(
				fmt.Errorf("invalid negative schema version provider configuration for provider %q", name),
			)
		}

		for t, r := range resp.ResourceTypes {
			s.ResourceTypes[t] = r.Block
			s.ResourceTypeSchemaVersions[t] = uint64(r.Version)
			if r.Version < 0 {
				diags = diags.Append(
					fmt.Errorf("invalid negative schema version for resource type %s in provider %q", t, name),
				)
			}
		}

		for t, d := range resp.DataSources {
			s.DataSources[t] = d.Block
			if d.Version < 0 {
				// We're not using the version numbers here yet, but we'll check
				// for validity anyway in case we start using them in future.
				diags = diags.Append(
					fmt.Errorf("invalid negative schema version for data source %s in provider %q", t, name),
				)
			}
		}

		schemas[fqn] = s

		if resp.ProviderMeta.Block != nil {
			s.ProviderMeta = resp.ProviderMeta.Block
		}
	}

	if config != nil {
		for _, fqn := range config.ProviderTypes() {
			ensure(fqn)
		}
	}

	if state != nil {
		needed := providers.AddressedTypesAbs(state.ProviderAddrs())
		for _, typeAddr := range needed {
			ensure(typeAddr)
		}
	}

	return diags
}

func loadProvisionerSchemas(schemas map[string]*configschema.Block, config *configs.Config, components contextComponentFactory) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	ensure := func(name string) {
		if _, exists := schemas[name]; exists {
			return
		}

		log.Printf("[TRACE] LoadSchemas: retrieving schema for provisioner %q", name)
		provisioner, err := components.ResourceProvisioner(name)
		if err != nil {
			// We'll put a stub in the map so we won't re-attempt this on
			// future calls.
			schemas[name] = &configschema.Block{}
			diags = diags.Append(
				fmt.Errorf("Failed to instantiate provisioner %q to obtain schema: %w", name, err),
			)
			return
		}
		defer func() {
			if closer, ok := provisioner.(ResourceProvisionerCloser); ok {
				closer.Close()
			}
		}()

		resp := provisioner.GetSchema()
		if resp.Diagnostics.HasErrors() {
			// We'll put a stub in the map so we won't re-attempt this on
			// future calls.
			schemas[name] = &configschema.Block{}
			diags = diags.Append(
				fmt.Errorf("Failed to retrieve schema from provisioner %q: %w", name, resp.Diagnostics.Err()),
			)
			return
		}

		schemas[name] = resp.Provisioner
	}

	if config != nil {
		for _, rc := range config.Module.ManagedResources {
			for _, pc := range rc.Managed.Provisioners {
				ensure(pc.Type)
			}
		}

		// Must also visit our child modules, recursively.
		for _, cc := range config.Children {
			childDiags := loadProvisionerSchemas(schemas, cc, components)
			diags = diags.Append(childDiags)
		}
	}

	return diags
}

// ProviderSchema represents the schema for a provider's own configuration
// and the configuration for some or all of its resources and data sources.
//
// The completeness of this structure depends on how it was constructed.
// When constructed for a configuration, it will generally include only
// resource types and data sources used by that configuration.
type ProviderSchema struct {
	Provider      *configschema.Block
	ProviderMeta  *configschema.Block
	ResourceTypes map[string]*configschema.Block
	DataSources   map[string]*configschema.Block

	ResourceTypeSchemaVersions map[string]uint64
}

// SchemaForResourceType attempts to find a schema for the given mode and type.
// Returns nil if no such schema is available.
func (ps *ProviderSchema) SchemaForResourceType(mode addrs.ResourceMode, typeName string) (schema *configschema.Block, version uint64) {
	switch mode {
	case addrs.ManagedResourceMode:
		return ps.ResourceTypes[typeName], ps.ResourceTypeSchemaVersions[typeName]
	case addrs.DataResourceMode:
		// Data resources don't have schema versions right now, since state is discarded for each refresh
		return ps.DataSources[typeName], 0
	default:
		// Shouldn't happen, because the above cases are comprehensive.
		return nil, 0
	}
}

// SchemaForResourceAddr attempts to find a schema for the mode and type from
// the given resource address. Returns nil if no such schema is available.
func (ps *ProviderSchema) SchemaForResourceAddr(addr addrs.Resource) (schema *configschema.Block, version uint64) {
	return ps.SchemaForResourceType(addr.Mode, addr.Type)
}

// ProviderSchemaRequest is used to describe to a ResourceProvider which
// aspects of schema are required, when calling the GetSchema method.
type ProviderSchemaRequest struct {
	ResourceTypes []string
	DataSources   []string
}
