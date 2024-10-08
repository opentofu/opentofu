// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/zclconf/go-cty/cty"
)

func TestNodeAbstractResourceProvider(t *testing.T) {
	tests := []struct {
		Addr   addrs.ConfigResource
		Config *configs.Resource
		Want   addrs.Provider
	}{
		{
			Addr: addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "null_resource",
				Name: "baz",
			}.InModule(addrs.RootModule),
			Want: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "hashicorp",
				Type:      "null",
			},
		},
		{
			Addr: addrs.Resource{
				Mode: addrs.DataResourceMode,
				Type: "terraform_remote_state",
				Name: "baz",
			}.InModule(addrs.RootModule),
			Want: addrs.Provider{
				// As a special case, the type prefix "terraform_" maps to
				// the builtin provider, not the default one.
				Hostname:  addrs.BuiltInProviderHost,
				Namespace: addrs.BuiltInProviderNamespace,
				Type:      "terraform",
			},
		},
		{
			Addr: addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "null_resource",
				Name: "baz",
			}.InModule(addrs.RootModule),
			Config: &configs.Resource{
				// Just enough configs.Resource for the Provider method. Not
				// actually valid for general use.
				Provider: addrs.Provider{
					Hostname:  addrs.DefaultProviderRegistryHost,
					Namespace: "awesomecorp",
					Type:      "happycloud",
				},
			},
			// The config overrides the default behavior.
			Want: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "awesomecorp",
				Type:      "happycloud",
			},
		},
		{
			Addr: addrs.Resource{
				Mode: addrs.DataResourceMode,
				Type: "terraform_remote_state",
				Name: "baz",
			}.InModule(addrs.RootModule),
			Config: &configs.Resource{
				// Just enough configs.Resource for the Provider method. Not
				// actually valid for general use.
				Provider: addrs.Provider{
					Hostname:  addrs.DefaultProviderRegistryHost,
					Namespace: "awesomecorp",
					Type:      "happycloud",
				},
			},
			// The config overrides the default behavior.
			Want: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "awesomecorp",
				Type:      "happycloud",
			},
		},
	}

	for _, test := range tests {
		var name string
		if test.Config != nil {
			name = fmt.Sprintf("%s with configured %s", test.Addr, test.Config.Provider)
		} else {
			name = fmt.Sprintf("%s with no configuration", test.Addr)
		}
		t.Run(name, func(t *testing.T) {
			node := &NodeAbstractResource{
				// Just enough NodeAbstractResource for the Provider function.
				// (This would not be valid for some other functions.)
				Addr:   test.Addr,
				Config: test.Config,
			}
			got := node.Provider()
			if got != test.Want {
				t.Errorf("wrong result\naddr:  %s\nconfig: %#v\ngot:   %s\nwant:  %s", test.Addr, test.Config, got, test.Want)
			}
		})
	}
}

func TestNodeAbstractResourceResolveInstanceProvider(t *testing.T) {
	applyableProvider := NodeApplyableProvider{
		NodeAbstractProvider: &NodeAbstractProvider{
			Addr: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/null"]`),
		},
	}

	tests := []struct {
		name               string
		instanceAddr       addrs.AbsResourceInstance
		potentialProviders []distinguishableProvider
		Want               addrs.AbsProviderConfig
	}{
		{
			name:         "potential provider pointing to a resource instance with an instance key",
			instanceAddr: mustResourceInstanceAddr("null_resource.resource[\"first\"]"),
			potentialProviders: []distinguishableProvider{{
				concreteProvider:   applyableProvider,
				resourceIdentifier: addrs.StringKey("first")}},
			Want: addrs.AbsProviderConfig{Provider: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "hashicorp",
				Type:      "null",
			}},
		},
		{
			name:         "potential provider pointing to a resource instance under a child module with an instance key",
			instanceAddr: mustResourceInstanceAddr("module.child_module[\"first\"].null_resource.resource"),
			potentialProviders: []distinguishableProvider{{
				concreteProvider: applyableProvider,
				moduleIdentifier: []addrs.ModuleInstanceStep{
					{
						Name:        "child_module",
						InstanceKey: addrs.StringKey("first"),
					},
				}}},
			Want: addrs.AbsProviderConfig{Provider: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "hashicorp",
				Type:      "null",
			}},
		},
		{
			name:         "potential provider pointing to a resource instance with an instance key under a child module with an instance key",
			instanceAddr: mustResourceInstanceAddr("module.child_module[\"first\"].null_resource.resource[\"first\"]"),
			potentialProviders: []distinguishableProvider{{
				concreteProvider:   applyableProvider,
				resourceIdentifier: addrs.StringKey("first"),
				moduleIdentifier: []addrs.ModuleInstanceStep{
					{
						Name:        "child_module",
						InstanceKey: addrs.StringKey("first"),
					},
				}}},
			Want: addrs.AbsProviderConfig{Provider: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "hashicorp",
				Type:      "null",
			}},
		},
		{
			name:         "potential provider pointing to a resource instance with an instance key under a nested child module with an instance key on the nested module",
			instanceAddr: mustResourceInstanceAddr("module.child_module.module.nested_module[\"first\"].null_resource.resource[\"first\"]"),
			potentialProviders: []distinguishableProvider{{
				concreteProvider:   applyableProvider,
				resourceIdentifier: addrs.StringKey("first"),
				moduleIdentifier: []addrs.ModuleInstanceStep{
					{
						Name: "child_module",
					},
					{
						Name:        "nested_module",
						InstanceKey: addrs.StringKey("first"),
					},
				}}},
			Want: addrs.AbsProviderConfig{Provider: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "hashicorp",
				Type:      "null",
			}},
		},
		{
			name:         "potential provider pointing to a resource instance with an instance key under a nested child module with an instance key on the nested module (with the instance key on the child module ignored)",
			instanceAddr: mustResourceInstanceAddr("module.child_module[\"ignored\"].module.nested_module[\"first\"].null_resource.resource[\"first\"]"),
			potentialProviders: []distinguishableProvider{{
				concreteProvider:   applyableProvider,
				resourceIdentifier: addrs.StringKey("first"),
				moduleIdentifier: []addrs.ModuleInstanceStep{
					{
						Name: "child_module",
					},
					{
						Name:        "nested_module",
						InstanceKey: addrs.StringKey("first"),
					},
				}}},
			Want: addrs.AbsProviderConfig{Provider: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "hashicorp",
				Type:      "null",
			}},
		},
		{
			name:         "potential provider pointing to a resource instance with an instance key under a nested child module with an instance key on both modules",
			instanceAddr: mustResourceInstanceAddr("module.child_module[\"first\"].module.nested_module[\"first\"].null_resource.resource[\"first\"]"),
			potentialProviders: []distinguishableProvider{{
				concreteProvider:   applyableProvider,
				resourceIdentifier: addrs.StringKey("first"),
				moduleIdentifier: []addrs.ModuleInstanceStep{
					{
						Name:        "child_module",
						InstanceKey: addrs.StringKey("first"),
					},
					{
						Name:        "nested_module",
						InstanceKey: addrs.StringKey("first"),
					},
				}}},
			Want: addrs.AbsProviderConfig{Provider: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "hashicorp",
				Type:      "null",
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := &NodeAbstractResource{
				// Just enough NodeAbstractResource for the resolveInstanceProvider function.
				// (This would not be valid for some other functions.)
				potentialProviders: test.potentialProviders,
			}
			got := node.resolveInstanceProvider(test.instanceAddr)
			if got.String() != test.Want.String() {
				t.Errorf("wrong result\ninstanceAddr:  %s\npotentialProviders: %#v\ngot:   %s\nwant:  %s", test.instanceAddr, test.potentialProviders, got, test.Want)
			}
		})
	}
}

// Make sure ProvideBy returns the final resolved provider
func TestNodeAbstractResourceSetProvider(t *testing.T) {
	node := &NodeAbstractResource{

		// Just enough NodeAbstractResource for the Provider function.
		// (This would not be valid for some other functions.)
		Addr: addrs.Resource{
			Mode: addrs.DataResourceMode,
			Type: "terraform_remote_state",
			Name: "baz",
		}.InModule(addrs.RootModule),
		Config: &configs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "terraform_remote_state",
			Name: "baz",
			// Just enough configs.Resource for the Provider method. Not
			// actually valid for general use.
			Provider: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "awesomecorp",
				Type:      "happycloud",
			},
		},
	}

	providersPerInstances, exact := node.ProvidedBy()
	if exact.provider.IsSet() {
		t.Fatalf("no exact provider should be found from this confniguration, got %q\n", exact.provider)
	}

	if len(providersPerInstances) > 1 {
		t.Fatalf("should have returned a single provider, got %d of providers instead\n", len(providersPerInstances))
	}

	p := providersPerInstances[addrs.NoKey]

	// the implied non-exact provider should be "terraform"
	lpc, ok := p.(addrs.LocalProviderConfig)
	if !ok {
		t.Fatalf("expected LocalProviderConfig, got %#v\n", p)
	}

	if lpc.LocalName != "terraform" {
		t.Fatalf("expected non-exact provider of 'terraform', got %q", lpc.LocalName)
	}

	// now set a resolved provider for the resource
	resolved := addrs.AbsProviderConfig{
		Provider: addrs.Provider{
			Hostname:  addrs.DefaultProviderRegistryHost,
			Namespace: "awesomecorp",
			Type:      "happycloud",
		},
		Module: addrs.RootModule,
		Alias:  "test",
	}

	node.SetProvider(resolved, true)

	providersPerInstances, exact = node.ProvidedBy()
	if !exact.provider.IsSet() {
		t.Fatalf("exact provider should be found, but it is empty\n")
	}

	p = exact.provider

	apc, ok := p.(addrs.AbsProviderConfig)
	if !ok {
		t.Fatalf("expected AbsProviderConfig, got %#v\n", p)
	}

	if apc.String() != resolved.String() {
		t.Fatalf("incorrect resolved config: got %#v, wanted %#v\n", apc, resolved)
	}
}

func TestNodeAbstractResource_ReadResourceInstanceState(t *testing.T) {
	mockProvider := mockProviderWithResourceTypeSchema("aws_instance", &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"id": {
				Type:     cty.String,
				Optional: true,
			},
		},
	})
	// This test does not configure the provider, but the mock provider will
	// check that this was called and report errors.
	mockProvider.ConfigureProviderCalled = true

	tests := map[string]struct {
		State              *states.State
		Node               *NodeAbstractResource
		ExpectedInstanceId string
	}{
		"ReadState gets primary instance state": {
			State: states.BuildState(func(s *states.SyncState) {
				providerAddr := addrs.AbsProviderConfig{
					Provider: addrs.NewDefaultProvider("aws"),
					Module:   addrs.RootModule,
				}
				oneAddr := addrs.Resource{
					Mode: addrs.ManagedResourceMode,
					Type: "aws_instance",
					Name: "bar",
				}.Absolute(addrs.RootModuleInstance)
				s.SetResourceProvider(oneAddr, providerAddr)
				s.SetResourceInstanceCurrent(oneAddr.Instance(addrs.NoKey), &states.ResourceInstanceObjectSrc{
					Status:    states.ObjectReady,
					AttrsJSON: []byte(`{"id":"i-abc123"}`),
				}, providerAddr, addrs.AbsProviderConfig{})
			}),
			Node: &NodeAbstractResource{
				Addr:                     mustConfigResourceAddr("aws_instance.bar"),
				ResolvedResourceProvider: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
			},
			ExpectedInstanceId: "i-abc123",
		},
	}

	for k, test := range tests {
		t.Run(k, func(t *testing.T) {
			ctx := new(MockEvalContext)
			ctx.StateState = test.State.SyncWrapper()
			ctx.PathPath = addrs.RootModuleInstance
			ctx.ProviderSchemaSchema = mockProvider.GetProviderSchema()

			ctx.ProviderProvider = providers.Interface(mockProvider)

			got, readDiags := test.Node.readResourceInstanceState(ctx, test.Node.Addr.Resource.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance), test.Node.ResolvedResourceProvider)
			if readDiags.HasErrors() {
				t.Fatalf("[%s] Got err: %#v", k, readDiags.Err())
			}

			expected := test.ExpectedInstanceId

			if !(got != nil && got.Value.GetAttr("id") == cty.StringVal(expected)) {
				t.Fatalf("[%s] Expected output with ID %#v, got: %#v", k, expected, got)
			}
		})
	}
}

func TestNodeAbstractResource_ReadResourceInstanceStateDeposed(t *testing.T) {
	mockProvider := mockProviderWithResourceTypeSchema("aws_instance", &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"id": {
				Type:     cty.String,
				Optional: true,
			},
		},
	})
	// This test does not configure the provider, but the mock provider will
	// check that this was called and report errors.
	mockProvider.ConfigureProviderCalled = true

	tests := map[string]struct {
		State              *states.State
		Node               *NodeAbstractResource
		ExpectedInstanceId string
	}{
		"ReadStateDeposed gets deposed instance": {
			State: states.BuildState(func(s *states.SyncState) {
				providerAddr := addrs.AbsProviderConfig{
					Provider: addrs.NewDefaultProvider("aws"),
					Module:   addrs.RootModule,
				}
				oneAddr := addrs.Resource{
					Mode: addrs.ManagedResourceMode,
					Type: "aws_instance",
					Name: "bar",
				}.Absolute(addrs.RootModuleInstance)
				s.SetResourceProvider(oneAddr, providerAddr)
				s.SetResourceInstanceDeposed(oneAddr.Instance(addrs.NoKey), states.DeposedKey("00000001"), &states.ResourceInstanceObjectSrc{
					Status:    states.ObjectReady,
					AttrsJSON: []byte(`{"id":"i-abc123"}`),
				}, providerAddr, addrs.AbsProviderConfig{})
			}),
			Node: &NodeAbstractResource{
				Addr:                     mustConfigResourceAddr("aws_instance.bar"),
				ResolvedResourceProvider: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
			},
			ExpectedInstanceId: "i-abc123",
		},
	}
	for k, test := range tests {
		t.Run(k, func(t *testing.T) {
			ctx := new(MockEvalContext)
			ctx.StateState = test.State.SyncWrapper()
			ctx.PathPath = addrs.RootModuleInstance
			ctx.ProviderSchemaSchema = mockProvider.GetProviderSchema()
			ctx.ProviderProvider = providers.Interface(mockProvider)

			key := states.DeposedKey("00000001") // shim from legacy state assigns 0th deposed index this key

			got, readDiags := test.Node.readResourceInstanceStateDeposed(ctx, test.Node.Addr.Resource.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance), key, test.Node.ResolvedResourceProvider)
			if readDiags.HasErrors() {
				t.Fatalf("[%s] Got err: %#v", k, readDiags.Err())
			}

			expected := test.ExpectedInstanceId

			if !(got != nil && got.Value.GetAttr("id") == cty.StringVal(expected)) {
				t.Fatalf("[%s] Expected output with ID %#v, got: %#v", k, expected, got)
			}
		})
	}
}
