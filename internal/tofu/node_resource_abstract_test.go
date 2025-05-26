// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"testing"

	"github.com/opentofu/opentofu/internal/refactoring"
	"github.com/opentofu/opentofu/internal/tfdiags"

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

type readResourceInstanceStateTest struct {
	Name               string
	State              *states.State
	Node               *NodeAbstractResourceInstance
	MoveResults        refactoring.MoveResults
	Provider           *MockProvider
	ExpectedInstanceId string
	WantErrorStr       string
}

func getMockProviderForReadResourceInstanceState() *MockProvider {
	return &MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			ResourceTypes: map[string]providers.Schema{
				"aws_instance": constructProviderSchemaForTesting(map[string]*configschema.Attribute{
					"id": {
						Type: cty.String,
					},
				}),
				"aws_instance0": constructProviderSchemaForTesting(map[string]*configschema.Attribute{
					"id": {
						Type: cty.String,
					},
				}),
			},
		},
	}
}

func getReadResourceInstanceStateTests(stateBuilder func(s *states.SyncState)) []readResourceInstanceStateTest {
	mockProvider := getMockProviderForReadResourceInstanceState()

	mockProviderWithStateChange := getMockProviderForReadResourceInstanceState()
	// Changes id to i-abc1234
	mockProviderWithStateChange.MoveResourceStateResponse = &providers.MoveResourceStateResponse{
		TargetState: cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("i-abc1234")}),
	}

	mockProviderWithMoveUnsupported := getMockProviderForReadResourceInstanceState()
	mockProviderWithMoveUnsupported.MoveResourceStateFn = func(req providers.MoveResourceStateRequest) providers.MoveResourceStateResponse {
		return providers.MoveResourceStateResponse{
			Diagnostics: tfdiags.Diagnostics{tfdiags.Sourceless(tfdiags.Error, "move not supported", "")},
		}
	}

	// This test does not configure the provider, but the mock provider will
	// check that this was called and report errors.
	mockProvider.ConfigureProviderCalled = true
	mockProviderWithStateChange.ConfigureProviderCalled = true
	mockProviderWithMoveUnsupported.ConfigureProviderCalled = true

	tests := []readResourceInstanceStateTest{
		{
			Name:     "gets primary instance state",
			Provider: mockProvider,
			State:    states.BuildState(stateBuilder),
			Node: &NodeAbstractResourceInstance{
				NodeAbstractResource: NodeAbstractResource{
					ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`)},
				},
				// Otherwise prevRunAddr fails, since we have no current Addr in the state
				Addr: mustResourceInstanceAddr("aws_instance.bar"),
			},
			ExpectedInstanceId: "i-abc123",
		},
		{
			Name:     "resource moved to another type without state change",
			Provider: mockProvider,
			MoveResults: refactoring.MoveResults{
				Changes: addrs.MakeMap[addrs.AbsResourceInstance, refactoring.MoveSuccess](
					addrs.MakeMapElem(mustResourceInstanceAddr("aws_instance.bar"), refactoring.MoveSuccess{
						From: mustResourceInstanceAddr("aws_instance0.baz"),
						To:   mustResourceInstanceAddr("aws_instance.bar"),
					})),
			},
			State: states.BuildState(stateBuilder),
			Node: &NodeAbstractResourceInstance{
				NodeAbstractResource: NodeAbstractResource{
					ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`)},
				},
				// Otherwise prevRunAddr fails, since we have no current Addr in the state
				Addr: mustResourceInstanceAddr("aws_instance.bar"),
			},
			ExpectedInstanceId: "i-abc123",
		},
		{
			Name:     "resource moved to another type with state change",
			Provider: mockProviderWithStateChange,
			MoveResults: refactoring.MoveResults{
				Changes: addrs.MakeMap[addrs.AbsResourceInstance, refactoring.MoveSuccess](
					addrs.MakeMapElem(mustResourceInstanceAddr("aws_instance.bar"), refactoring.MoveSuccess{
						From: mustResourceInstanceAddr("aws_instance0.baz"),
						To:   mustResourceInstanceAddr("aws_instance.bar"),
					})),
			},
			State: states.BuildState(stateBuilder),
			Node: &NodeAbstractResourceInstance{
				NodeAbstractResource: NodeAbstractResource{
					ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`)},
				},
				// Otherwise prevRunAddr fails, since we have no current Addr in the state
				Addr: mustResourceInstanceAddr("aws_instance.bar"),
			},
			// The state change should have been applied
			ExpectedInstanceId: "i-abc1234",
		},
		{
			Name:     "resource moved to another type but move not supported by provider",
			Provider: mockProviderWithMoveUnsupported,
			MoveResults: refactoring.MoveResults{
				Changes: addrs.MakeMap[addrs.AbsResourceInstance, refactoring.MoveSuccess](
					addrs.MakeMapElem(mustResourceInstanceAddr("aws_instance.bar"), refactoring.MoveSuccess{
						From: mustResourceInstanceAddr("aws_instance0.baz"),
					}),
				),
			},
			State: states.BuildState(stateBuilder),
			Node: &NodeAbstractResourceInstance{
				NodeAbstractResource: NodeAbstractResource{
					ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`)},
				},
				Addr: mustResourceInstanceAddr("aws_instance.bar"),
			},
			WantErrorStr: "move not supported",
		},
	}
	return tests
}

// TestNodeAbstractResource_ReadResourceInstanceState tests the readResourceInstanceState and readResourceInstanceStateDeposed methods of NodeAbstractResource.
// Those are quite similar in behavior, so we can test them together.
func TestNodeAbstractResource_ReadResourceInstanceState(t *testing.T) {
	tests := getReadResourceInstanceStateTests(func(s *states.SyncState) {
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
	})
	for _, test := range tests {
		t.Run("ReadState "+test.Name, func(t *testing.T) {
			evalCtx := new(MockEvalContext)
			evalCtx.StateState = test.State.SyncWrapper()
			evalCtx.PathPath = addrs.RootModuleInstance
			evalCtx.ProviderSchemaSchema = test.Provider.GetProviderSchema(t.Context())
			evalCtx.MoveResultsResults = test.MoveResults
			evalCtx.ProviderProvider = providers.Interface(test.Provider)

			got, readDiags := test.Node.readResourceInstanceState(t.Context(), evalCtx, test.Node.Addr)
			if test.WantErrorStr != "" {
				if !readDiags.HasErrors() {
					t.Fatalf("[%s] Expected error, got none", test.Name)
				}
				if readDiags.Err().Error() != test.WantErrorStr {
					t.Fatalf("[%s] Expected error %q, got %q", test.Name, test.WantErrorStr, readDiags.Err().Error())
				}
				return
			}
			if readDiags.HasErrors() {
				t.Fatalf("[%s] Got err: %#v", test.Name, readDiags.Err())
			}

			expected := test.ExpectedInstanceId

			if got == nil || got.Value.GetAttr("id") != cty.StringVal(expected) {
				t.Fatalf("[%s] Expected output with ID %#v, got: %#v", test.Name, expected, got)
			}
		})
	}
	// Deposed tests
	deposedTests := getReadResourceInstanceStateTests(func(s *states.SyncState) {
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
		s.SetResourceInstanceDeposed(oneAddr.Instance(addrs.NoKey), "00000001", &states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"i-abc123"}`),
		}, providerAddr, addrs.NoKey)
	})
	for _, test := range deposedTests {
		t.Run("ReadStateDeposed "+test.Name, func(t *testing.T) {
			evalCtx := new(MockEvalContext)
			evalCtx.StateState = test.State.SyncWrapper()
			evalCtx.PathPath = addrs.RootModuleInstance
			evalCtx.ProviderSchemaSchema = test.Provider.GetProviderSchema(t.Context())
			evalCtx.MoveResultsResults = test.MoveResults
			evalCtx.ProviderProvider = providers.Interface(test.Provider)

			key := states.DeposedKey("00000001") // shim from legacy state assigns 0th deposed index this key
			got, readDiags := test.Node.readResourceInstanceStateDeposed(t.Context(), evalCtx, test.Node.Addr, key)
			if test.WantErrorStr != "" {
				if !readDiags.HasErrors() {
					t.Fatalf("[%s] Expected error, got none", test.Name)
				}
				if readDiags.Err().Error() != test.WantErrorStr {
					t.Fatalf("[%s] Expected error %q, got %q", test.Name, test.WantErrorStr, readDiags.Err().Error())
				}
				return
			}
			if readDiags.HasErrors() {
				t.Fatalf("[%s] Got err: %#v", test.Name, readDiags.Err())
			}

			expected := test.ExpectedInstanceId

			if got == nil || got.Value.GetAttr("id") != cty.StringVal(expected) {
				t.Fatalf("[%s] Expected output with ID %#v, got: %#v", test.Name, expected, got)
			}
		})

	}
}
