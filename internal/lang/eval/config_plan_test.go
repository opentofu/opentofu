// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval_test

import (
	"context"
	"errors"
	"iter"
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
	planResult, diags := configInst.DrivePlanning(t.Context(), func(oracle *eval.PlanningOracle) eval.PlanGlue {
		logGlue.oracle = oracle
		return logGlue
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	gotOutputs := planResult.RootModuleOutputs
	wantOutputs := cty.ObjectVal(map[string]cty.Value{
		"c": cty.StringVal("true:true/true:true"),
	})
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
	planResult, diags := configInst.DrivePlanning(t.Context(), func(oracle *eval.PlanningOracle) eval.PlanGlue {
		logGlue.oracle = oracle
		return logGlue
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	gotOutputs := planResult.RootModuleOutputs
	wantOutputs := cty.ObjectVal(map[string]cty.Value{
		"c": cty.StringVal("foo bar name"),
	})
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
			Provider: addrs.MustParseProviderSourceString("test/foo"),
			ProviderInstance: &addrs.AbsProviderInstanceCorrect{
				Config: addrs.AbsProviderConfigCorrect{
					Config: addrs.ProviderConfigCorrect{
						Provider: addrs.MustParseProviderSourceString("test/foo"),
					},
				},
			},
		}),
	)
	if diff := cmp.Diff(wantReqs, gotReqs, ctydebug.CmpOptions); diff != "" {
		t.Error("wrong requests\n" + diff)
	}

	providerInstAddr := addrs.AbsProviderInstanceCorrect{
		Config: addrs.AbsProviderConfigCorrect{
			Config: addrs.ProviderConfigCorrect{
				Provider: addrs.MustParseProviderSourceString("test/foo"),
			},
		},
	}
	gotProviderInstConfigs := logGlue.providerInstanceConfigs
	wantProviderInstConfigs := addrs.MakeMap(
		addrs.MakeMapElem(providerInstAddr, cty.ObjectVal(map[string]cty.Value{
			"greeting": cty.StringVal("Hello"),
		})),
	)
	if diff := cmp.Diff(wantProviderInstConfigs, gotProviderInstConfigs, ctydebug.CmpOptions); diff != "" {
		t.Error("wrong provider instance configs\n" + diff)
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
	planResult, diags := configInst.DrivePlanning(t.Context(), func(oracle *eval.PlanningOracle) eval.PlanGlue {
		logGlue.oracle = oracle
		return logGlue
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	gotOutputs := planResult.RootModuleOutputs
	wantOutputs := cty.ObjectVal(map[string]cty.Value{
		"c": cty.DynamicVal, // don't know what instances we have yet
	})
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
			Provider: addrs.MustParseProviderSourceString("test/foo"),
			ProviderInstance: &addrs.AbsProviderInstanceCorrect{
				Config: addrs.AbsProviderConfigCorrect{
					Config: addrs.ProviderConfigCorrect{
						Provider: addrs.MustParseProviderSourceString("test/foo"),
					},
				},
			},
		}),
	)
	if diff := cmp.Diff(wantReqs, gotReqs, ctydebug.CmpOptions); diff != "" {
		t.Error("wrong requests\n" + diff)
	}
}

type planGlueCallLog struct {
	oracle    *eval.PlanningOracle
	providers eval.ProvidersSchema

	resourceInstanceRequests addrs.Map[addrs.AbsResourceInstance, *eval.DesiredResourceInstance]
	providerInstanceConfigs  addrs.Map[addrs.AbsProviderInstanceCorrect, cty.Value]
	mu                       sync.Mutex
}

// ValidateProviderConfig implements eval.PlanGlue
func (p *planGlueCallLog) ValidateProviderConfig(ctx context.Context, provider addrs.Provider, configVal cty.Value) tfdiags.Diagnostics {
	return nil
}

// PlanDesiredResourceInstance implements eval.PlanGlue.
func (p *planGlueCallLog) PlanDesiredResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance) (cty.Value, tfdiags.Diagnostics) {
	p.mu.Lock()
	if p.resourceInstanceRequests.Len() == 0 {
		p.resourceInstanceRequests = addrs.MakeMap[addrs.AbsResourceInstance, *eval.DesiredResourceInstance]()
	}
	p.resourceInstanceRequests.Put(inst.Addr, inst)
	if inst.ProviderInstance != nil {
		if p.providerInstanceConfigs.Len() == 0 {
			p.providerInstanceConfigs = addrs.MakeMap[addrs.AbsProviderInstanceCorrect, cty.Value]()
		}
		providerInstAddr := *inst.ProviderInstance
		providerInstConfig := p.oracle.ProviderInstanceConfig(ctx, providerInstAddr)
		p.providerInstanceConfigs.Put(providerInstAddr, providerInstConfig)
	}
	p.mu.Unlock()

	if p.providers == nil {
		var diags tfdiags.Diagnostics
		diags = diags.Append(errors.New("cannot use resources in this test without including an eval.Providers object to the planGlueCallLog object"))
		return cty.DynamicVal, diags
	}
	schema, diags := p.providers.ResourceTypeSchema(ctx, inst.Provider, inst.Addr.Resource.Resource.Mode, inst.Addr.Resource.Resource.Type)
	if diags.HasErrors() {
		return cty.DynamicVal, diags
	}
	plannedVal := objchange.ProposedNew(schema.Block, cty.NullVal(schema.Block.ImpliedType()), inst.ConfigVal)
	return plannedVal, diags
}

// PlanModuleCallInstanceOrphans implements eval.PlanGlue.
func (p *planGlueCallLog) PlanModuleCallInstanceOrphans(ctx context.Context, moduleCallAddr addrs.AbsModuleCall, desiredInstances iter.Seq[addrs.InstanceKey]) tfdiags.Diagnostics {
	// We don't currently do anything with calls to this method, because
	// no tests we've written so far rely on it.
	return nil
}

// PlanModuleCallOrphans implements eval.PlanGlue.
func (p *planGlueCallLog) PlanModuleCallOrphans(ctx context.Context, callerModuleInstAddr addrs.ModuleInstance, desiredCalls iter.Seq[addrs.ModuleCall]) tfdiags.Diagnostics {
	// We don't currently do anything with calls to this method, because
	// no tests we've written so far rely on it.
	return nil
}

// PlanResourceInstanceOrphans implements eval.PlanGlue.
func (p *planGlueCallLog) PlanResourceInstanceOrphans(ctx context.Context, resourceAddr addrs.AbsResource, desiredInstances iter.Seq[addrs.InstanceKey]) tfdiags.Diagnostics {
	// We don't currently do anything with calls to this method, because
	// no tests we've written so far rely on it.
	return nil
}

// PlanResourceOrphans implements eval.PlanGlue.
func (p *planGlueCallLog) PlanResourceOrphans(ctx context.Context, moduleInstAddr addrs.ModuleInstance, desiredResources iter.Seq[addrs.Resource]) tfdiags.Diagnostics {
	// We don't currently do anything with calls to this method, because
	// no tests we've written so far rely on it.
	return nil
}
