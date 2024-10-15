// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/states"
)

func TestNodeAbstractResourceInstanceProvider(t *testing.T) {
	tests := []struct {
		Addr                 addrs.AbsResourceInstance
		Config               *configs.Resource
		StoredProviderConfig addrs.AbsProviderConfig
		Want                 addrs.Provider
	}{
		{
			Addr: addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "null_resource",
				Name: "baz",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
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
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
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
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
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
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
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
				Type: "null_resource",
				Name: "baz",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Config: nil,
			StoredProviderConfig: addrs.AbsProviderConfig{
				Module: addrs.RootModule,
				Provider: addrs.Provider{
					Hostname:  addrs.DefaultProviderRegistryHost,
					Namespace: "awesomecorp",
					Type:      "null",
				},
			},
			// The stored provider config overrides the default behavior.
			Want: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "awesomecorp",
				Type:      "null",
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
			node := &NodeAbstractResourceInstance{
				// Just enough NodeAbstractResourceInstance for the Provider
				// function. (This would not be valid for some other functions.)
				Addr: test.Addr,
				NodeAbstractResource: NodeAbstractResource{
					Addr:   test.Addr.ConfigResource(),
					Config: test.Config,
				},
				storedProviderConfig: test.StoredProviderConfig,
			}
			got := node.Provider()
			if got != test.Want {
				t.Errorf("wrong result\naddr:  %s\nconfig: %#v\ngot:   %s\nwant:  %s", test.Addr, test.Config, got, test.Want)
			}
		})
	}
}

func TestNodeAbstractResourceInstance_WriteResourceInstanceState(t *testing.T) {
	state := states.NewState()
	ctx := new(MockEvalContext)
	ctx.StateState = state.SyncWrapper()
	ctx.PathPath = addrs.RootModuleInstance

	mockProvider := mockProviderWithResourceTypeSchema("aws_instance", &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"id": {
				Type:     cty.String,
				Optional: true,
			},
		},
	})

	obj := &states.ResourceInstanceObject{
		Value: cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("i-abc123"),
		}),
		Status: states.ObjectReady,
	}

	node := &NodeAbstractResourceInstance{
		Addr: mustResourceInstanceAddr("aws_instance.foo"),
		// instanceState:        obj,
		NodeAbstractResource:     NodeAbstractResource{},
		ResolvedInstanceProvider: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
	}
	ctx.ProviderProvider = mockProvider
	ctx.ProviderSchemaSchema = mockProvider.GetProviderSchema()

	err := node.writeResourceInstanceState(ctx, obj, workingState)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	checkStateString(t, state, `
aws_instance.foo:
  ID = i-abc123
  provider = provider["registry.opentofu.org/hashicorp/aws"]
	`)
}

/* TODO
func TestNodeAbstractResourceInstance_ResolvedProvider(t *testing.T) {
	provider := mustProviderConfig(`provider["registry.opentofu.org/hashicorp/null"]`)

	tests := []struct {
		name                     string
		ResolvedResourceProvider addrs.AbsProviderConfig
		ResolvedInstanceProvider addrs.AbsProviderConfig
		expectPanic              bool
		expectedPanicMessage     string
	}{
		{
			name:                     "ResolvedResourceProvider is set",
			ResolvedResourceProvider: provider,
			ResolvedInstanceProvider: addrs.AbsProviderConfig{},
		},
		{
			name:                     "ResolvedInstanceProvider is set",
			ResolvedResourceProvider: addrs.AbsProviderConfig{},
			ResolvedInstanceProvider: provider,
		},
		{
			name:                     "Panic if both providers are set",
			ResolvedResourceProvider: provider,
			ResolvedInstanceProvider: provider,
			expectPanic:              true,
			expectedPanicMessage:     "ResolvedProvider for null_resource.resource has a provider set for the resource and the resource's instance",
		},
		{
			name:                     "Panic if no provider set",
			ResolvedResourceProvider: addrs.AbsProviderConfig{},
			ResolvedInstanceProvider: addrs.AbsProviderConfig{},
			expectPanic:              true,
			expectedPanicMessage:     "ResolvedProvider for null_resource.resource cannot get a provider",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			n := &NodeAbstractResourceInstance{
				NodeAbstractResource: NodeAbstractResource{
					ResolvedResourceProvider: test.ResolvedResourceProvider,
				},
				ResolvedInstanceProvider: test.ResolvedInstanceProvider,
				Addr:                     mustResourceInstanceAddr("null_resource.resource"),
			}

			if test.expectPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("Expected panic, but got none")
					} else if r != test.expectedPanicMessage {
						t.Errorf("Expected panic message '%s', but got '%v'", test.expectedPanicMessage, r)
					}
				}()
				n.ResolvedProvider()
			} else {
				result := n.ResolvedProvider()
				if !result.IsSet() {
					t.Errorf("Expected provider to be set, but it was not")
				}
				if result.String() != provider.String() {
					t.Errorf("Expected provider %v, but got %v", provider, result)
				}
			}
		})
	}
}*/
