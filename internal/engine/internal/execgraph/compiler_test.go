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
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
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
	//
	// FIXME: This test became more gnarly and less maintainable as a result
	// of the recent changes to the execgraph operations, but so far we've
	// just done the minimum to make it work again. In future commits we should
	// find a different way to test this that doesn't require so much
	// boilerplate and mocking.
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
	providerInstAddrRef := builder.ConstantProviderInstAddr(providerInstAddr)
	providerInstConfig := builder.ProviderInstanceConfig(providerInstAddrRef, nil)
	providerClient := builder.ProviderInstanceOpen(providerInstConfig)
	providerCloseDeps, addProviderUser := builder.MutableWaiter()
	_ = builder.ProviderInstanceClose(providerClient, providerCloseDeps)
	instAddrResult := builder.ConstantResourceInstAddr(resourceInstAddr)
	desiredInst := builder.ResourceInstanceDesired(instAddrResult, nil)
	priorState := builder.ResourceInstancePrior(instAddrResult)
	finalPlan := builder.ManagedFinalPlan(
		desiredInst,
		priorState,
		initialPlannedValue,
		providerClient,
	)
	newState := builder.ManagedApply(
		finalPlan,
		NilResultRef[*exec.ResourceInstanceObject](),
		providerClient,
		nil,
	)
	addProviderUser(newState)
	builder.SetResourceInstanceFinalStateResult(resourceInstAddr, newState)
	sourceGraph := builder.Finish()
	t.Log("source graph:\n" + sourceGraph.DebugRepr())

	// The rest of this is approximating what the apply phase might do, although
	// only the part that relates to this package in particular since we're
	// focused only on testing the compiler and our ability to execute what
	// it produces.
	var ops *mockOperations
	ops = &mockOperations{
		ResourceInstanceDesiredFunc: func(ctx context.Context, addr addrs.AbsResourceInstance) (*eval.DesiredResourceInstance, tfdiags.Diagnostics) {
			if !addr.Equal(resourceInstAddr) {
				return nil, nil
			}
			return &eval.DesiredResourceInstance{
				Addr: addr,
				ConfigVal: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("thingy"),
				}),
				Provider:         providerAddr,
				ProviderInstance: &providerInstAddr,
				ResourceMode:     addrs.ManagedResourceMode,
				ResourceType:     addr.Resource.Resource.Type,
			}, nil
		},
		ResourceInstancePriorFunc: func(ctx context.Context, addr addrs.AbsResourceInstance) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
			return &exec.ResourceInstanceObject{
				InstanceAddr: addr,
				State: &states.ResourceInstanceObjectFull{
					Status: states.ObjectReady,
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("prior"),
					}),
					ProviderInstanceAddr: addrs.AbsProviderInstanceCorrect{
						Config: addrs.AbsProviderConfigCorrect{
							Config: addrs.ProviderConfigCorrect{
								Provider: addrs.NewBuiltInProvider("test"),
							},
						},
					},
					ResourceType: addr.Resource.Resource.Type,
				},
			}, nil
		},
		ManagedFinalPlanFunc: func(ctx context.Context, desired *eval.DesiredResourceInstance, prior *exec.ResourceInstanceObject, plannedVal cty.Value, providerClient *exec.ProviderClient) (*exec.ManagedResourceObjectFinalPlan, tfdiags.Diagnostics) {
			return &exec.ManagedResourceObjectFinalPlan{
				InstanceAddr:  desired.Addr,
				ResourceType:  desired.ResourceType,
				ConfigVal:     desired.ConfigVal,
				PriorStateVal: prior.State.Value,
				PlannedVal:    plannedVal,
			}, nil
		},
		ManagedApplyFunc: func(ctx context.Context, plan *exec.ManagedResourceObjectFinalPlan, fallback *exec.ResourceInstanceObject, providerClient *exec.ProviderClient) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
			return &exec.ResourceInstanceObject{
				InstanceAddr: plan.InstanceAddr,
				State: &states.ResourceInstanceObjectFull{
					Status:               states.ObjectReady,
					Value:                plan.PlannedVal,
					ResourceType:         plan.ResourceType,
					ProviderInstanceAddr: providerClient.InstanceAddr,
				},
			}, nil
		},
		ProviderInstanceConfigFunc: func(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) (*exec.ProviderInstanceConfig, tfdiags.Diagnostics) {
			if !addr.Equal(providerInstAddr) {
				return nil, nil
			}
			return &exec.ProviderInstanceConfig{
				InstanceAddr: addr,
				ConfigVal: cty.ObjectVal(map[string]cty.Value{
					"provider_config": cty.True,
				}),
			}, nil
		},
		ProviderInstanceOpenFunc: func(ctx context.Context, config *exec.ProviderInstanceConfig) (*exec.ProviderClient, tfdiags.Diagnostics) {
			return &exec.ProviderClient{
				InstanceAddr: config.InstanceAddr,
				Ops: ops.NewManagedResourceProviderClient(
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
				),
			}, nil
		},
	}
	compiledGraph, diags := sourceGraph.Compile(ops)
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

	gotLog := ops.Calls
	// There are multiple valid call orders, so we'll just discard the order
	// by sorting the log by method name since we only expect one call to
	// each method in this particular test.
	slices.SortFunc(gotLog, func(a, b mockOperationsCall) int {
		return cmp.Compare(a.MethodName, b.MethodName)
	})
	// We also can't compare the actual provider client, so we'll stub that
	// result out.
	for i := range gotLog {
		// We can't reliably compare the actual provider clients and so
		// we'll just replace them with their instance addresses.
		for ai, arg := range gotLog[i].Args {
			if c, ok := arg.(*exec.ProviderClient); ok {
				gotLog[i].Args[ai] = c.InstanceAddr
			}
		}
		if c, ok := gotLog[i].Result.(*exec.ProviderClient); ok {
			gotLog[i].Result = c.InstanceAddr
		}
	}
	wantLog := []mockOperationsCall{
		{
			MethodName: "ManagedApply",
			Args: []any{
				&exec.ManagedResourceObjectFinalPlan{
					InstanceAddr: resourceInstAddr,
					ResourceType: resourceInstAddr.Resource.Resource.Type,
					ConfigVal:    wantValue,
					PlannedVal:   wantValue,
					PriorStateVal: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("prior"),
					}),
				},
				(*exec.ResourceInstanceObject)(nil),
				providerInstAddr,
			},
			Result: &exec.ResourceInstanceObject{
				InstanceAddr: resourceInstAddr,
				State: &states.ResourceInstanceObjectFull{
					Status:               states.ObjectReady,
					Value:                wantValue,
					ProviderInstanceAddr: providerInstAddr,
					ResourceType:         resourceInstAddr.Resource.Resource.Type,
				},
			},
		},
		{
			MethodName: "ManagedFinalPlan",
			Args: []any{
				&eval.DesiredResourceInstance{
					Addr:             resourceInstAddr,
					ConfigVal:        wantValue,
					Provider:         providerAddr,
					ProviderInstance: &providerInstAddr,
					ResourceMode:     addrs.ManagedResourceMode,
					ResourceType:     resourceInstAddr.Resource.Resource.Type,
				},
				&exec.ResourceInstanceObject{
					InstanceAddr: resourceInstAddr,
					State: &states.ResourceInstanceObjectFull{
						Status: states.ObjectReady,
						Value: cty.ObjectVal(map[string]cty.Value{
							"name": cty.StringVal("prior"),
						}),
						ProviderInstanceAddr: addrs.AbsProviderInstanceCorrect{
							Config: addrs.AbsProviderConfigCorrect{
								Config: addrs.ProviderConfigCorrect{
									Provider: addrs.NewBuiltInProvider("test"),
								},
							},
						},
						ResourceType: resourceInstAddr.Resource.Resource.Type,
					},
				},
				wantValue,
				providerInstAddr,
			},
			Result: &exec.ManagedResourceObjectFinalPlan{
				InstanceAddr: resourceInstAddr,
				ResourceType: resourceInstAddr.Resource.Resource.Type,
				ConfigVal:    wantValue,
				PriorStateVal: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("prior"),
				}),
				PlannedVal: wantValue,
			},
		},
		{
			MethodName: "ProviderInstanceClose",
			Args: []any{
				providerInstAddr,
			},
			Result: struct{}{},
		},
		{
			MethodName: "ProviderInstanceConfig",
			Args: []any{
				providerInstAddr,
			},
			Result: &exec.ProviderInstanceConfig{
				InstanceAddr: providerInstAddr,
				ConfigVal: cty.ObjectVal(map[string]cty.Value{
					"provider_config": cty.True,
				}),
			},
		},
		{
			MethodName: "ProviderInstanceOpen",
			Args: []any{
				&exec.ProviderInstanceConfig{
					InstanceAddr: providerInstAddr,
					ConfigVal: cty.ObjectVal(map[string]cty.Value{
						"provider_config": cty.True,
					}),
				},
			},
			Result: providerInstAddr,
		},
		{
			MethodName: "ResourceInstanceDesired",
			Args: []any{
				resourceInstAddr,
			},
			Result: &eval.DesiredResourceInstance{
				Addr:             resourceInstAddr,
				ConfigVal:        wantValue,
				Provider:         providerAddr,
				ProviderInstance: &providerInstAddr,
				ResourceMode:     addrs.ManagedResourceMode,
				ResourceType:     resourceInstAddr.Resource.Resource.Type,
			},
		},
		{
			MethodName: "ResourceInstancePrior",
			Args: []any{
				resourceInstAddr,
			},
			Result: &exec.ResourceInstanceObject{
				InstanceAddr: resourceInstAddr,
				State: &states.ResourceInstanceObjectFull{
					Status: states.ObjectReady,
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("prior"),
					}),
					ProviderInstanceAddr: addrs.AbsProviderInstanceCorrect{
						Config: addrs.AbsProviderConfigCorrect{
							Config: addrs.ProviderConfigCorrect{
								Provider: addrs.NewBuiltInProvider("test"),
							},
						},
					},
					ResourceType: resourceInstAddr.Resource.Resource.Type,
				},
			},
		},
	}
	if diff := gcmp.Diff(wantLog, gotLog, ctydebug.CmpOptions); diff != "" {
		t.Errorf("wrong ExecContext calls: %s", diff)
	}
}
