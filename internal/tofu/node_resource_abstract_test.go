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

	p := node.ProvidedBy()

	// the implied non-exact provider should be "terraform"
	lpc, ok := p.ProviderConfig.(addrs.LocalProviderConfig)
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

	node.SetProvider(ResolvedProvider{ProviderConfig: resolved})
	p = node.ProvidedBy()

	apc, ok := p.ProviderConfig.(addrs.AbsProviderConfig)
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
		Node               *NodeAbstractResourceInstance
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
				}, providerAddr, addrs.NoKey)
			}),
			Node: &NodeAbstractResourceInstance{
				NodeAbstractResource: NodeAbstractResource{
					ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`)},
				},
				//Otherwise prevRunAddr fails, since we have no current Addr in the state
				Addr: mustResourceInstanceAddr("aws_instance.bar"),
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
			got, readDiags := test.Node.readResourceInstanceState(ctx, test.Node.Addr)
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
		Node               *NodeAbstractResourceInstance
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
				}, providerAddr, addrs.NoKey)
			}),
			Node: &NodeAbstractResourceInstance{
				NodeAbstractResource: NodeAbstractResource{
					ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`)},
				},
				Addr: mustResourceInstanceAddr("aws_instance.bar"),
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

			got, readDiags := test.Node.readResourceInstanceStateDeposed(ctx, test.Node.Addr, key)
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
