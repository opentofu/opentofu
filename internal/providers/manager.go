package providers

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
)

type SchemaCacheFn func(supplier func() ProviderSchema) ProviderSchema

type Manager interface {
	Schemas
	NewProviderInstance(addr addrs.Provider) (Interface, error)
}

//TODO type SchemaFilter func(addrs.ResourceMode, string) bool

type manager struct {
	schemas   map[addrs.Provider]ProviderSchema
	factories map[addrs.Provider]Factory

	// FUTURE: extend to take over responsibilities of managing provider interfaces from BuiltinEvalContext
}

func NewManager(ctx context.Context, factories map[addrs.Provider]Factory) (Manager, error) {
	m := &manager{
		schemas:   map[addrs.Provider]ProviderSchema{},
		factories: factories,
	}

	var errs []error
	// TODO parallelize
	for addr := range factories {

		// Initialize
		log.Printf("[TRACE] providers.Manager: Initializing provider %q to read its schema", addr)
		instance, err := m.NewProviderInstance(addr)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to initialize provider %q: %w", addr, err))
			continue
		}

		// Pre-load the schemas
		resp := instance.GetProviderSchema(ctx)
		m.schemas[addr] = resp

		err = instance.Close(ctx)
		if err != nil {
			errs = append(errs, err)
		}

		// Validate
		if resp.Diagnostics.HasErrors() {
			errs = append(errs, fmt.Errorf("failed to retrieve schema from provider %q: %w", addr, resp.Diagnostics.Err()))
		}

		if resp.Provider.Version < 0 {
			// We're not using the version numbers here yet, but we'll check
			// for validity anyway in case we start using them in future.
			errs = append(errs, fmt.Errorf("provider %s has invalid negative schema version for its configuration blocks,which is a bug in the provider ", addr))
		}

		for t, r := range resp.ResourceTypes {
			if err := r.Block.InternalValidate(); err != nil {
				errs = append(errs, fmt.Errorf("provider %s has invalid schema for managed resource type %q, which is a bug in the provider: %w", addr, t, err))
			}
			if r.Version < 0 {
				errs = append(errs, fmt.Errorf("provider %s has invalid negative schema version for managed resource type %q, which is a bug in the provider", addr, t))
			}
		}

		for t, d := range resp.DataSources {
			if err := d.Block.InternalValidate(); err != nil {
				errs = append(errs, fmt.Errorf("provider %s has invalid schema for data resource type %q, which is a bug in the provider: %w", addr, t, err))
			}
			if d.Version < 0 {
				// We're not using the version numbers here yet, but we'll check
				// for validity anyway in case we start using them in future.
				errs = append(errs, fmt.Errorf("provider %s has invalid negative schema version for data resource type %q, which is a bug in the provider", addr, t))
			}
		}
	}

	return m, errors.Join(errs...)
}

func (m *manager) HasProvider(addr addrs.Provider) bool {
	_, ok := m.factories[addr]
	return ok
}

func (m *manager) NewProviderInstance(addr addrs.Provider) (Interface, error) {
	f, ok := m.factories[addr]
	if !ok {
		return nil, fmt.Errorf("unavailable provider %q", addr.String())
	}

	return f(func(supplier func() ProviderSchema) ProviderSchema {
		if schema, ok := m.schemas[addr]; ok {
			log.Printf("[TRACE] providers.Manager: Serving provider %q schema from global schema cache", addr)
			return schema
		}
		log.Printf("[TRACE] providers.Manager: Fetching provider %q schema", addr)
		m.schemas[addr] = supplier()
		return m.schemas[addr]
	})

}

func (m *manager) ProviderSchemas() map[addrs.Provider]ProviderSchema {
	// TODO copy this for safety
	return m.schemas
}

func (m *manager) ProviderSchema(addr addrs.Provider) (ProviderSchema, error) {
	schema, ok := m.schemas[addr]
	if !ok {
		return schema, fmt.Errorf("unavailable provider %q", addr.String())
	}
	return schema, nil
}

// ProviderConfigSchema is a helper wrapper around ProviderSchema which first
// reads the full schema of the given provider and then extracts just the
// provider's configuration schema, which defines what's expected in a
// "provider" block in the configuration when configuring this provider.
func (m *manager) ProviderConfigSchema(providerAddr addrs.Provider) (*configschema.Block, error) {
	providerSchema, err := m.ProviderSchema(providerAddr)
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
func (m *manager) ResourceTypeSchema(providerAddr addrs.Provider, resourceMode addrs.ResourceMode, resourceType string) (*configschema.Block, uint64, error) {
	providerSchema, err := m.ProviderSchema(providerAddr)
	if err != nil {
		return nil, 0, err
	}

	schema, version := providerSchema.SchemaForResourceType(resourceMode, resourceType)
	return schema, version, nil
}
