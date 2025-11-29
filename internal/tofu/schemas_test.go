// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
)

// TestResourceTypeConfig checks that ResourceTypeConfig works correctly in all possible combinations:
// * Provider has schema for all types (managed && data && ephemeral)
// * Provider has schema for only 2 types (managed && data or managed && ephemeral or data && ephemeral)
// * Provider has schema for only 1 type (managed or data or ephemeral)
//
// This is due to a check that was in [ResourceTypeConfig] that could have created issues
// but was guarded by an initialisation of the [providers.ProviderSchema.ResourceTypes] during
// proto.GetProviderSchema (both, for protov5 and protov6).
func TestResourceTypeConfig(t *testing.T) {
	type resCheck struct {
		resourceMode addrs.ResourceMode
		resourceType string
		exist        bool
	}

	cases := map[string]struct {
		schema       *Schemas
		providerAddr addrs.Provider
		checks       []resCheck
	}{
		"all types": {
			schema: &Schemas{
				Providers: map[addrs.Provider]providers.ProviderSchema{
					addrs.NewDefaultProvider("test"): {
						ResourceTypes: map[string]providers.Schema{
							"testmanaged": {Block: &configschema.Block{}},
						},
						DataSources: map[string]providers.Schema{
							"testdata": {Block: &configschema.Block{}},
						},
						EphemeralResources: map[string]providers.Schema{
							"testephemeral": {Block: &configschema.Block{}},
						},
					},
				},
			},
			providerAddr: addrs.NewDefaultProvider("test"),
			checks: []resCheck{
				{
					resourceMode: addrs.ManagedResourceMode,
					resourceType: "testmanaged",
					exist:        true,
				},
				{
					resourceMode: addrs.DataResourceMode,
					resourceType: "testdata",
					exist:        true,
				},
				{
					resourceMode: addrs.EphemeralResourceMode,
					resourceType: "testephemeral",
					exist:        true,
				},
			},
		},
		"only managed and data": {
			schema: &Schemas{
				Providers: map[addrs.Provider]providers.ProviderSchema{
					addrs.NewDefaultProvider("test"): {
						ResourceTypes: map[string]providers.Schema{
							"testmanaged": {Block: &configschema.Block{}},
						},
						DataSources: map[string]providers.Schema{
							"testdata": {Block: &configschema.Block{}},
						},
					},
				},
			},
			providerAddr: addrs.NewDefaultProvider("test"),
			checks: []resCheck{
				{
					resourceMode: addrs.ManagedResourceMode,
					resourceType: "testmanaged",
					exist:        true,
				},
				{
					resourceMode: addrs.DataResourceMode,
					resourceType: "testdata",
					exist:        true,
				},
				{
					resourceMode: addrs.EphemeralResourceMode,
					resourceType: "testephemeral",
					exist:        false,
				},
			},
		},
		"only managed and ephemeral": {
			schema: &Schemas{
				Providers: map[addrs.Provider]providers.ProviderSchema{
					addrs.NewDefaultProvider("test"): {
						ResourceTypes: map[string]providers.Schema{
							"testmanaged": {Block: &configschema.Block{}},
						},
						EphemeralResources: map[string]providers.Schema{
							"testephemeral": {Block: &configschema.Block{}},
						},
					},
				},
			},
			providerAddr: addrs.NewDefaultProvider("test"),
			checks: []resCheck{
				{
					resourceMode: addrs.ManagedResourceMode,
					resourceType: "testmanaged",
					exist:        true,
				},
				{
					resourceMode: addrs.DataResourceMode,
					resourceType: "testdata",
					exist:        false,
				},
				{
					resourceMode: addrs.EphemeralResourceMode,
					resourceType: "testephemeral",
					exist:        true,
				},
			},
		},
		"only data and ephemeral": {
			schema: &Schemas{
				Providers: map[addrs.Provider]providers.ProviderSchema{
					addrs.NewDefaultProvider("test"): {
						DataSources: map[string]providers.Schema{
							"testdata": {Block: &configschema.Block{}},
						},
						EphemeralResources: map[string]providers.Schema{
							"testephemeral": {Block: &configschema.Block{}},
						},
					},
				},
			},
			providerAddr: addrs.NewDefaultProvider("test"),
			checks: []resCheck{
				{
					resourceMode: addrs.ManagedResourceMode,
					resourceType: "testmanaged",
					exist:        false,
				},
				{
					resourceMode: addrs.DataResourceMode,
					resourceType: "testdata",
					exist:        true,
				},
				{
					resourceMode: addrs.EphemeralResourceMode,
					resourceType: "testephemeral",
					exist:        true,
				},
			},
		},
		"only managed": {
			schema: &Schemas{
				Providers: map[addrs.Provider]providers.ProviderSchema{
					addrs.NewDefaultProvider("test"): {
						ResourceTypes: map[string]providers.Schema{
							"testmanaged": {Block: &configschema.Block{}},
						},
					},
				},
			},
			providerAddr: addrs.NewDefaultProvider("test"),
			checks: []resCheck{
				{
					resourceMode: addrs.ManagedResourceMode,
					resourceType: "testmanaged",
					exist:        true,
				},
				{
					resourceMode: addrs.DataResourceMode,
					resourceType: "testdata",
					exist:        false,
				},
				{
					resourceMode: addrs.EphemeralResourceMode,
					resourceType: "testephemeral",
					exist:        false,
				},
			},
		},
		"only data": {
			schema: &Schemas{
				Providers: map[addrs.Provider]providers.ProviderSchema{
					addrs.NewDefaultProvider("test"): {
						DataSources: map[string]providers.Schema{
							"testdata": {Block: &configschema.Block{}},
						},
					},
				},
			},
			providerAddr: addrs.NewDefaultProvider("test"),
			checks: []resCheck{
				{
					resourceMode: addrs.ManagedResourceMode,
					resourceType: "testmanaged",
					exist:        false,
				},
				{
					resourceMode: addrs.DataResourceMode,
					resourceType: "testdata",
					exist:        true,
				},
				{
					resourceMode: addrs.EphemeralResourceMode,
					resourceType: "testephemeral",
					exist:        false,
				},
			},
		},
		"only ephemeral": {
			schema: &Schemas{
				Providers: map[addrs.Provider]providers.ProviderSchema{
					addrs.NewDefaultProvider("test"): {
						EphemeralResources: map[string]providers.Schema{
							"testephemeral": {Block: &configschema.Block{}},
						},
					},
				},
			},
			providerAddr: addrs.NewDefaultProvider("test"),
			checks: []resCheck{
				{
					resourceMode: addrs.ManagedResourceMode,
					resourceType: "testmanaged",
					exist:        false,
				},
				{
					resourceMode: addrs.DataResourceMode,
					resourceType: "testdata",
					exist:        false,
				},
				{
					resourceMode: addrs.EphemeralResourceMode,
					resourceType: "testephemeral",
					exist:        true,
				},
			},
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			for _, check := range tt.checks {
				checkTestName := fmt.Sprintf("%s:%s.%s", name, check.resourceMode.String(), check.resourceType)
				t.Run(checkTestName, func(t *testing.T) {
					b, _ := tt.schema.ResourceTypeConfig(tt.providerAddr, check.resourceMode, check.resourceType)
					if b == nil && check.exist {
						t.Fatalf("expected to have a schema for resource mode %q and type %q for the provider %q but got nothing. schema:\n%+v", check.resourceMode, check.resourceType, tt.providerAddr, tt.schema)
					} else if b != nil && !check.exist {
						t.Fatalf("expected to have no schema for resource mode %q and type %q for the provider %q but got one. schema:\n%+v", check.resourceMode, check.resourceType, tt.providerAddr, tt.schema)
					}
				})
			}
		})
	}
}

func simpleTestSchemas() *Schemas {
	provider := simpleMockProvider()
	provisioner := simpleMockProvisioner()

	return &Schemas{
		Providers: map[addrs.Provider]providers.ProviderSchema{
			addrs.NewDefaultProvider("test"): provider.GetProviderSchema(context.TODO()),
		},
		Provisioners: map[string]*configschema.Block{
			"test": provisioner.GetSchemaResponse.Provisioner,
		},
	}
}

// schemaOnlyProvidersForTesting is a testing helper that constructs a
// plugin library that contains a set of providers that only know how to
// return schema, and will exhibit undefined behavior if used for any other
// purpose.
//
// The intended use for this is in testing components that use schemas to
// drive other behavior, such as reference analysis during graph construction,
// but that don't actually need to interact with providers otherwise.
func schemaOnlyProvidersForTesting(schemas map[addrs.Provider]providers.ProviderSchema, t *testing.T) *contextPlugins {
	factories := make(map[addrs.Provider]providers.Factory, len(schemas))

	for providerAddr, schema := range schemas {
		schema := schema

		// mark ephemeral resources blocks accordingly
		for _, s := range schema.EphemeralResources {
			s.Block.Ephemeral = true
		}
		provider := &MockProvider{
			GetProviderSchemaResponse: &schema,
		}

		factories[providerAddr] = func() (providers.Interface, error) {
			return provider, nil
		}
	}

	return newContextPlugins(factories, nil)
}
