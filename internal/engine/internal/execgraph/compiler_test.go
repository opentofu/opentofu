// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"testing"

	gcmp "github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestCompiler_resourceInstanceBasics(t *testing.T) {
	// The following approximates might appear in the planning engine's code
	// for building the execution subgraph for a desired resource instance,
	// arranging for its changes to be planned and applied with whatever
	// provider instance was selected in the configuration.
	builder := NewBuilder()
	resourceInstAddr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "bar_thing",
		Name: "example",
	}.Absolute(addrs.RootModuleInstance).Instance(addrs.NoKey)
	providerAddr := addrs.MustParseProviderSourceString("example.com/foo/bar")
	providerInstAddr := addrs.AbsProviderInstanceCorrect{
		Config: addrs.AbsProviderConfigCorrect{
			Config: addrs.ProviderConfigCorrect{
				Provider: providerAddr,
			},
		},
	}
	initialPlannedValue := builder.ConstantValue(cty.ObjectVal(map[string]cty.Value{
		"name": cty.StringVal("thingy"),
	}))
	providerClient, addProviderUser := builder.ProviderInstance(providerInstAddr, nil)
	desiredInst := builder.DesiredResourceInstance(resourceInstAddr)
	priorState := builder.ResourceInstancePriorState(resourceInstAddr)
	finalPlan := builder.ManagedResourceObjectFinalPlan(
		desiredInst,
		priorState,
		initialPlannedValue,
		providerClient,
		nil,
	)
	newState := builder.ApplyManagedResourceObjectChanges(finalPlan, providerClient)
	addProviderUser(newState)
	builder.SetResourceInstanceFinalStateResult(resourceInstAddr, newState)
	sourceGraph := builder.Finish()
	t.Log("source graph:\n" + sourceGraph.DebugRepr())

	// The rest of this is approximating what the apply phase might do, although
	// only the part that relates to this package in particular since we're
	// focused only on testing the compiler and our ability to execute what
	// it produces.
	var execCtx *MockExecContext
	execCtx = &MockExecContext{
		DesiredResourceInstanceFunc: func(ctx context.Context, addr addrs.AbsResourceInstance) *eval.DesiredResourceInstance {
			if !addr.Equal(resourceInstAddr) {
				return nil
			}
			return &eval.DesiredResourceInstance{
				Addr: addr,
				ConfigVal: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("thingy"),
				}),
				Provider:         providerAddr,
				ProviderInstance: &providerInstAddr,
			}
		},
		ResourceInstancePriorStateFunc: func(ctx context.Context, addr addrs.AbsResourceInstance, deposedKey states.DeposedKey) *states.ResourceInstanceObject {
			return &states.ResourceInstanceObject{
				Status: states.ObjectReady,
				Value: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("prior"),
				}),
			}
		},
		ProviderInstanceConfigFunc: func(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) cty.Value {
			if !addr.Equal(providerInstAddr) {
				return cty.NilVal
			}
			return cty.ObjectVal(map[string]cty.Value{
				"provider_config": cty.True,
			})
		},
		NewProviderClientFunc: func(ctx context.Context, addr addrs.Provider, configVal cty.Value) (providers.Configured, tfdiags.Diagnostics) {
			return execCtx.NewManagedResourceProviderClient(
				func(ctx context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
					return providers.PlanResourceChangeResponse{
						PlannedState: req.Config,
					}
				},
				func(ctx context.Context, req providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
					return providers.ApplyResourceChangeResponse{
						NewState: req.PlannedState,
					}
				},
			), nil
		},
	}
	compiledGraph, diags := sourceGraph.Compile(execCtx)
	if diags.HasErrors() {
		t.Fatal("unexpected compile errors\n" + diags.Err().Error())
	}

	var wg sync.WaitGroup
	diagsCh := make(chan tfdiags.Diagnostics, 1)
	wg.Go(func() {
		diagsCh <- compiledGraph.Execute(t.Context())
		close(diagsCh)
	})

	gotValue := compiledGraph.ResourceInstanceValue(grapheval.ContextWithNewWorker(t.Context()), resourceInstAddr)
	wantValue := cty.ObjectVal(map[string]cty.Value{
		"name": cty.StringVal("thingy"),
	})
	if diff := gcmp.Diff(wantValue, gotValue, ctydebug.CmpOptions); diff != "" {
		t.Errorf("wrong result for %s: %s", resourceInstAddr, diff)
	}

	wg.Wait()
	diags = <-diagsCh
	if diags.HasErrors() {
		t.Fatal("unexpected execute errors\n" + diags.Err().Error())
	}

	gotLog := execCtx.Calls
	// There are multiple valid call orders, so we'll just discard the order
	// by sorting the log by method name since we only expect one call to
	// each method in this particular test.
	slices.SortFunc(gotLog, func(a, b MockExecContextCall) int {
		return cmp.Compare(a.MethodName, b.MethodName)
	})
	// We also can't compare the actual provider client, so we'll stub that
	// result out.
	for i := range gotLog {
		if gotLog[i].MethodName == "NewProviderClient" {
			gotLog[i].Result = "<ignored for test comparison>"
		}
	}
	wantLog := []MockExecContextCall{
		{
			MethodName: "DesiredResourceInstance",
			Args:       []any{resourceInstAddr},
			Result: &eval.DesiredResourceInstance{
				Addr: resourceInstAddr,
				ConfigVal: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("thingy"),
				}),
				Provider:         providerAddr,
				ProviderInstance: &providerInstAddr,
			},
		},
		{
			MethodName: "NewProviderClient",
			Args: []any{
				providerAddr,
				cty.ObjectVal(map[string]cty.Value{
					"provider_config": cty.True,
				}),
			},
			Result: "<ignored for test comparison>",
		},
		{
			MethodName: "ProviderInstanceConfig",
			Args:       []any{providerInstAddr},
			Result: cty.ObjectVal(map[string]cty.Value{
				"provider_config": cty.True,
			}),
		},
		{
			MethodName: "ResourceInstancePriorState",
			Args:       []any{resourceInstAddr, states.NotDeposed},
			Result: &states.ResourceInstanceObject{
				Status: states.ObjectReady,
				Value: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("prior"),
				}),
			},
		},
		{
			MethodName: "providerClient.ApplyResourceChange",
			Args: []any{
				providers.ApplyResourceChangeRequest{
					TypeName: "bar_thing",
					PriorState: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("prior"),
					}),
					PlannedState: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("thingy"),
					}),
					Config: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("thingy"),
					}),
				},
			},
			Result: providers.ApplyResourceChangeResponse{
				NewState: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("thingy"),
				}),
			},
		},
		{
			MethodName: "providerClient.PlanResourceChange",
			Args: []any{
				providers.PlanResourceChangeRequest{
					TypeName: "bar_thing",
					PriorState: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("prior"),
					}),
					Config: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("thingy"),
					}),
				},
			},
			Result: providers.PlanResourceChangeResponse{
				PlannedState: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("thingy"),
				}),
			},
		},
	}
	if diff := gcmp.Diff(wantLog, gotLog, ctydebug.CmpOptions); diff != "" {
		t.Errorf("wrong ExecContext calls: %s", diff)
	}
}
