// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tofu/testhelpers"
	"github.com/zclconf/go-cty/cty"
)

// mockProviderWithConfigSchema is a test helper to concisely create a mock
// provider with the given schema for its own configuration.
func mockProviderWithConfigSchema(schema *configschema.Block) *testhelpers.MockProvider {
	return &testhelpers.MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			Provider: providers.Schema{Block: schema},
		},
	}
}

// mockProviderWithResourceTypeSchema is a test helper to concisely create a mock
// provider with a schema containing a single resource type.
func mockProviderWithResourceTypeSchema(name string, schema *configschema.Block) *testhelpers.MockProvider {
	return &testhelpers.MockProvider{
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
