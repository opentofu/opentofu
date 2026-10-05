// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/plans/objchange"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// This file is in "package eval_test" in order to integration-test the
// validation phase through the same exported API that external callers would
// use.

// Simulate the same steps the planning engine takes
func planEngineSimulator(t *testing.T, configInst *eval.ConfigInstance, logGlue *planGlueCallLog) (map[string]cty.Value, tfdiags.Diagnostics) {
	oracle, ctx, diags := configInst.BuildPlanningOracle(t.Context(), logGlue)
	if diags.HasErrors() {
		return nil, diags
	}

	diags = oracle.CheckAll(ctx)
	if diags.HasErrors() {
		return nil, diags
	}

	planResult := oracle.PlanningResult(ctx)

	gotOutputs := map[string]cty.Value{}
	for name, val := range planResult.RootModuleOutputs {
		gotOutputs[name] = val.Value
	}

	return gotOutputs, diags
}

func TestPlan_valuesOnlySuccess(t *testing.T) {
	// This test has an intentionally limited scope covering just the
	// basics, so that we don't necessarily need to repeat these basics
	// across all of the other tests.

	configInst, diags := eval.NewConfigInstance(t.Context(), &eval.ConfigCall{
		EvalContext: evalglue.EvalContextForTesting(t, &eval.EvalContext{
			Modules: eval.ModulesForTesting(map[addrs.ModuleSourceLocal]*configs.Module{
				addrs.ModuleSourceLocal("."): configs.ModuleFromStringForTesting(t, `
					variable "a" {
						type = string
					}
					locals {
						b = "${var.a}:${var.a}"
					}
					output "c" {
						value = "${local.b}/${local.b}"
					}
				`),
			}),
		}),
		RootModuleSource: addrs.ModuleSourceLocal("."),
		InputValues: eval.InputValuesForTesting(map[string]cty.Value{
			"a": cty.True,
		}),
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	logGlue := &planGlueCallLog{}
	gotOutputs, diags := planEngineSimulator(t, configInst, logGlue)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}
	wantOutputs := map[string]cty.Value{
		"c": cty.StringVal("true:true/true:true"),
	}
	if diff := cmp.Diff(wantOutputs, gotOutputs, ctydebug.CmpOptions); diff != "" {
		t.Error("wrong result\n" + diff)
	}
}

func TestPlan_managedResourceSimple(t *testing.T) {
	// This test has an intentionally limited scope covering just the
	// basics, so that we don't necessarily need to repeat these basics
	// across all of the other tests.

	providers := eval.ProvidersForTesting(map[addrs.Provider]*providers.GetProviderSchemaResponse{
		addrs.MustParseProviderSourceString("test/foo"): {
			Provider: providers.Schema{
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"greeting": {
							Type:     cty.String,
							Required: true,
						},
					},
				},
			},
			ResourceTypes: map[string]providers.Schema{
				"foo": {
					Block: &configschema.Block{
						Attributes: map[string]*configschema.Attribute{
							"name": {
								Type:     cty.String,
								Required: true,
							},
						},
					},
				},
			},
		},
	})
	configInst, diags := eval.NewConfigInstance(t.Context(), &eval.ConfigCall{
		EvalContext: evalglue.EvalContextForTesting(t, &eval.EvalContext{
			Modules: eval.ModulesForTesting(map[addrs.ModuleSourceLocal]*configs.Module{
				addrs.ModuleSourceLocal("."): configs.ModuleFromStringForTesting(t, `
					terraform {
						required_providers {
							foo = {
								source = "test/foo"
							}
						}
					}
					provider "foo" {
						greeting = "Hello"
					}
					variable "a" {
						type = string
					}
					resource "foo" "bar" {
						name = var.a
					}
					output "c" {
						value = foo.bar.name
					}
				`),
			}),
			Providers: providers,
		}),
		RootModuleSource: addrs.ModuleSourceLocal("."),
		InputValues: eval.InputValuesForTesting(map[string]cty.Value{
			"a": cty.StringVal("foo bar name"),
		}),
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	logGlue := &planGlueCallLog{
		providers: providers,
	}
	gotOutputs, diags := planEngineSimulator(t, configInst, logGlue)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}
	wantOutputs := map[string]cty.Value{
		"c": cty.StringVal("foo bar name"),
	}
	if diff := cmp.Diff(wantOutputs, gotOutputs, ctydebug.CmpOptions); diff != "" {
		t.Error("wrong result\n" + diff)
	}

	instAddr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "foo",
		Name: "bar",
	}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)
	gotReqs := logGlue.resourceInstanceRequests
	wantReqs := addrs.MakeMap(
		addrs.MakeMapElem(instAddr, &eval.DesiredResourceInstance{
			Addr: instAddr,
			ConfigVal: cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("foo bar name"),
			}),
			RequiredResourceInstances: addrs.MakeSet[addrs.AbsResourceInstance](),
		}),
	)
	if diff := cmp.Diff(wantReqs, gotReqs, ctydebug.CmpOptions); diff != "" {
		t.Error("wrong requests\n" + diff)
	}
}

func TestPlan_managedResourceUnknownCount(t *testing.T) {
	// This test has an intentionally limited scope covering just the
	// basics, so that we don't necessarily need to repeat these basics
	// across all of the other tests.

	providers := eval.ProvidersForTesting(map[addrs.Provider]*providers.GetProviderSchemaResponse{
		addrs.MustParseProviderSourceString("test/foo"): {
			ResourceTypes: map[string]providers.Schema{
				"foo": {
					Block: &configschema.Block{
						Attributes: map[string]*configschema.Attribute{
							"name": {
								Type:     cty.String,
								Required: true,
							},
						},
					},
				},
			},
		},
	})
	configInst, diags := eval.NewConfigInstance(t.Context(), &eval.ConfigCall{
		EvalContext: evalglue.EvalContextForTesting(t, &eval.EvalContext{
			Modules: eval.ModulesForTesting(map[addrs.ModuleSourceLocal]*configs.Module{
				addrs.ModuleSourceLocal("."): configs.ModuleFromStringForTesting(t, `
					terraform {
						required_providers {
							foo = {
								source = "test/foo"
							}
						}
					}
					variable "a" {
						type = string
					}
					variable "num" {
						type = number
					}
					resource "foo" "bar" {
						count = var.num

						name = var.a
					}
					output "c" {
						value = foo.bar[*].name
					}
				`),
			}),
			Providers: providers,
		}),
		RootModuleSource: addrs.ModuleSourceLocal("."),
		InputValues: eval.InputValuesForTesting(map[string]cty.Value{
			"a":   cty.StringVal("foo bar name"),
			"num": cty.UnknownVal(cty.Number),
		}),
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	logGlue := &planGlueCallLog{
		providers: providers,
	}
	gotOutputs, diags := planEngineSimulator(t, configInst, logGlue)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}
	wantOutputs := map[string]cty.Value{
		"c": cty.DynamicVal, // don't know what instances we have yet
	}
	if diff := cmp.Diff(wantOutputs, gotOutputs, ctydebug.CmpOptions); diff != "" {
		t.Error("wrong result\n" + diff)
	}

	// Because count is unknown, we plan a placeholder resource instance
	// whose instance key is a wildcard.
	instAddr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "foo",
		Name: "bar",
	}.Instance(addrs.WildcardKey{addrs.IntKeyType}).Absolute(addrs.RootModuleInstance)
	gotReqs := logGlue.resourceInstanceRequests
	wantReqs := addrs.MakeMap(
		addrs.MakeMapElem(instAddr, &eval.DesiredResourceInstance{
			Addr: instAddr,
			ConfigVal: cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("foo bar name"),
			}),
			RequiredResourceInstances: addrs.MakeSet[addrs.AbsResourceInstance](),
		}),
	)
	if diff := cmp.Diff(wantReqs, gotReqs, ctydebug.CmpOptions); diff != "" {
		t.Error("wrong requests\n" + diff)
	}
}

type planGlueCallLog struct {
	providers eval.ProvidersSchema

	resourceInstanceRequests addrs.Map[addrs.AbsResourceInstance, *eval.DesiredResourceInstance]
	mu                       sync.Mutex
}

// PreProcess implements eval.PlanGlue
func (p *planGlueCallLog) PreProcess(ctx context.Context, oracle *eval.PlanningOracle, targeter func(addrs.Targetable)) tfdiags.Diagnostics {
	// We don't currently do anything with calls to this method, because
	// no tests we've written so far rely on it.
	return nil
}

// PlanDesiredResourceInstance implements eval.PlanGlue.
func (p *planGlueCallLog) PlanDesiredResourceInstance(ctx context.Context, oracle *eval.PlanningOracle, inst *eval.DesiredResourceInstance) (cty.Value, tfdiags.Diagnostics) {
	p.mu.Lock()
	if p.resourceInstanceRequests.Len() == 0 {
		p.resourceInstanceRequests = addrs.MakeMap[addrs.AbsResourceInstance, *eval.DesiredResourceInstance]()
	}
	p.resourceInstanceRequests.Put(inst.Addr, inst)
	p.mu.Unlock()

	if p.providers == nil {
		var diags tfdiags.Diagnostics
		diags = diags.Append(errors.New("cannot use resources in this test without including an eval.Providers object to the planGlueCallLog object"))
		return cty.DynamicVal, diags
	}
	meta := oracle.ResourceInstanceObjectMeta(ctx, inst.Addr.CurrentObject())
	if meta == nil {
		var diags tfdiags.Diagnostics
		diags = diags.Append(fmt.Errorf("no resource instance object metadata for desired object %s", inst.Addr))
		return cty.DynamicVal, diags
	}
	schema, diags := p.providers.ResourceTypeSchema(ctx, meta.Provider, inst.Addr.Resource.Resource.Mode, inst.Addr.Resource.Resource.Type)
	if diags.HasErrors() {
		return cty.DynamicVal, diags
	}
	plannedVal := objchange.ProposedNew(schema.Block, cty.NullVal(schema.Block.ImpliedType()), inst.ConfigVal)
	return plannedVal, diags
}

// PostProcess implements eval.PlanGlue.
func (p *planGlueCallLog) PostProcess(ctx context.Context, oracle *eval.PlanningOracle) tfdiags.Diagnostics {
	// We don't currently do anything with calls to this method, because
	// no tests we've written so far rely on it.
	return nil
}
