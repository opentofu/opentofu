// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
)

// mockProviderWithConfigSchema is a test helper to concisely create a mock
// provider with the given schema for its own configuration.
func mockProviderWithConfigSchema(schema *configschema.Block) *MockProvider {
	return &MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			Provider: providers.Schema{Block: schema},
		},
	}
}

// mockProviderWithResourceTypeSchema is a test helper to concisely create a mock
// provider with a schema containing a single resource type.
func mockProviderWithResourceTypeSchema(name string, schema *configschema.Block) *MockProvider {
	return &MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			Provider: providers.Schema{
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"string": {
							Type:     cty.String,
							Optional: true,
						},
						"list": {
							Type:     cty.List(cty.String),
							Optional: true,
						},
						"root": {
							Type:     cty.Map(cty.String),
							Optional: true,
						},
					},
				},
			},
			ResourceTypes: map[string]providers.Schema{
				name: providers.Schema{Block: schema},
			},
		},
	}
}

// simpleMockProvider returns a MockProvider that is pre-configured
// with schema for its own config, for a resource type called "test_object" and
// for a data source also called "test_object".
//
// All three schemas have the same content as returned by function
// simpleTestSchema.
//
// For most reasonable uses the returned provider must be registered in a
// componentFactory under the name "test". Use simpleMockComponentFactory
// to obtain a pre-configured componentFactory containing the result of
// this function along with simpleMockProvisioner, both registered as "test".
//
// The returned provider has no other behaviors by default, but the caller may
// modify it in order to stub any other required functionality, or modify
// the default schema stored in the field GetSchemaReturn. Each new call to
// simpleTestProvider produces entirely new instances of all of the nested
// objects so that callers can mutate without affecting mock objects.
func simpleMockProvider() *MockProvider {
	return &MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			Provider: providers.Schema{Block: simpleTestSchema()},
			ResourceTypes: map[string]providers.Schema{
				"test_object": {Block: simpleTestSchema()},
			},
			DataSources: map[string]providers.Schema{
				"test_object": {Block: simpleTestSchema()},
			},
			EphemeralResources: map[string]providers.Schema{
				"test_object": {Block: simpleTestSchema()},
			},
		},
	}
}

// ProviderSchema is a helper to convert from the internal GetProviderSchemaResponse to
// a ProviderSchema.
func (p *MockProvider) ProviderSchema() *ProviderSchema {
	resp := p.getProviderSchema()

	schema := &ProviderSchema{
		Provider:                   resp.Provider.Block,
		ProviderMeta:               resp.ProviderMeta.Block,
		ResourceTypes:              map[string]*configschema.Block{},
		DataSources:                map[string]*configschema.Block{},
		EphemeralTypes:             map[string]*configschema.Block{},
		ResourceTypeSchemaVersions: map[string]uint64{},
	}

	for resType, s := range resp.ResourceTypes {
		schema.ResourceTypes[resType] = s.Block
		schema.ResourceTypeSchemaVersions[resType] = uint64(s.Version)
	}

	for dataSource, s := range resp.DataSources {
		schema.DataSources[dataSource] = s.Block
	}

	for ephemeral, s := range resp.EphemeralResources {
		schema.EphemeralTypes[ephemeral] = s.Block
	}

	return schema
}

// the type was refactored out with all the functionality handled within the
// provider package, but we keep this here for a shim in existing tests.
type ProviderSchema struct {
	Provider                   *configschema.Block
	ProviderMeta               *configschema.Block
	ResourceTypes              map[string]*configschema.Block
	ResourceTypeSchemaVersions map[string]uint64
	DataSources                map[string]*configschema.Block
	EphemeralTypes             map[string]*configschema.Block
}

// getProviderSchemaResponseFromProviderSchema is a test helper to convert a
// ProviderSchema to a GetProviderSchemaResponse for use when building a mock provider.
func getProviderSchemaResponseFromProviderSchema(providerSchema *ProviderSchema) *providers.GetProviderSchemaResponse {
	resp := &providers.GetProviderSchemaResponse{
		Provider:           providers.Schema{Block: providerSchema.Provider},
		ProviderMeta:       providers.Schema{Block: providerSchema.ProviderMeta},
		ResourceTypes:      map[string]providers.Schema{},
		DataSources:        map[string]providers.Schema{},
		EphemeralResources: map[string]providers.Schema{},
	}

	for name, schema := range providerSchema.ResourceTypes {
		resp.ResourceTypes[name] = providers.Schema{
			Block:   schema,
			Version: int64(providerSchema.ResourceTypeSchemaVersions[name]),
		}
	}

	for name, schema := range providerSchema.DataSources {
		resp.DataSources[name] = providers.Schema{Block: schema}
	}

	for name, schema := range providerSchema.EphemeralTypes {
		resp.EphemeralResources[name] = providers.Schema{Block: schema}
	}

	return resp
}
