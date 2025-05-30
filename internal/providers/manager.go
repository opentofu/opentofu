package providers

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
)

type Manager interface {
	Schemas
	NewProviderInstance(addr addrs.Provider) (Interface, error)
}

type manager struct {
	factories map[addrs.Provider]Factory

	// FUTURE: extend to take over responsibilities of managing provider interfaces from BuiltinEvalContext
}

func NewManager(factories map[addrs.Provider]Factory) Manager {
	return &manager{factories: factories}
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

	return f.Instance()

}

func (m *manager) ProviderSchemas() map[addrs.Provider]ProviderSchema {
	schemas := make(map[addrs.Provider]ProviderSchema, len(m.factories))
	for addr, factory := range m.factories {
		schemas[addr] = factory.Schema()
	}

	return schemas
}

func (m *manager) ProviderSchema(addr addrs.Provider) (ProviderSchema, error) {
	factory, ok := m.factories[addr]
	if !ok {
		return ProviderSchema{}, fmt.Errorf("unavailable provider %q", addr.String())
	}
	return factory.Schema(), nil
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
