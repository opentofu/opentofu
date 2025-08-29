// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"strconv"
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
)

// This test file contains a small collection of benchmarks, written using the benchmark
// mechanism offered as part of Go's testing library, of situations involving resources
// and modules that have a very large number of instances.
//
// OpenTofu's current design is aimed to support tens of instances as the typical case
// and low-hundreds of instances as an extreme case. These benchmarks intentionally
// ignore those design assumptions by testing with thousands of resource instances,
// since we know that some in our community use OpenTofu in that way and although it
// not officially supported we do wish to be able to more easily measure performance
// when someone reports a significant regression of performance when using an
// "unreasonable" number of instances (per OpenTofu's current design assumptions),
// or whenever we're intentionally attempting to change something in OpenTofu to
// improve performance.
//
// The existence of these benchmarks does not represent a commitment to support
// using OpenTofu with thousands of resource instances in the same configuration.
// We consider these situations to be "best effort" only.
//
// These benchmarks exercise the core language runtime only. Therefore they do not
// account for any additional overheads caused by behaviors at the CLI layer, such
// as remote state storage and the state snapshot serialization that implies, or
// the UI display hooks.

// This benchmark takes, at the time of writing, over a minute to perform just one
// iteration. Therefore at present it's best to just let it run once:
//
//	go test ./internal/tofu -bench='^BenchmarkManyResourceInstances$' -benchtime=1x
func BenchmarkManyResourceInstances(b *testing.B) {
	// instanceCount is the number of instances we declare _for each resource_.
	// Since there are two resources, there are 2*instanceCount instances total.
	const instanceCount = 2500
	m := testModuleInline(b, map[string]string{
		"main.tf": `
			# This test has two resources that each have a lot of instances
			# that are correlated with one another.

			terraform {
				required_providers {
					test = {
						source = "terraform.io/builtin/test"
					}
				}
			}

			variable "instance_count" {
				type = number
			}

			resource "test" "a" {
				count = var.instance_count

				num = count.index
			}

			resource "test" "b" {
				count = length(test.a)

				num = test.a[count.index].num
			}
		`,
	})
	p := &MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			ResourceTypes: map[string]providers.Schema{
				"test": {
					Block: &configschema.Block{
						Attributes: map[string]*configschema.Attribute{
							"num": {
								Type:     cty.Number,
								Required: true,
							},
						},
					},
				},
			},
		},
		PlanResourceChangeFn: func(prcr providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
			return providers.PlanResourceChangeResponse{
				PlannedState: prcr.ProposedNewState,
			}
		},
		ApplyResourceChangeFn: func(arcr providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
			return providers.ApplyResourceChangeResponse{
				NewState: arcr.PlannedState,
			}
		},
	}
	tofuCtx := testContext2(b, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewBuiltInProvider("test"): testProviderFuncFixed(p),
		},
		// With this many resource instances we need a high concurrency limit
		// for the runtime to be in any way reasonable. In this case we're
		// going to set it so high that there is effectively no limit at all,
		// which measures a best-case scenario where we're limited only by
		// OpenTofu's direct overheads and not by the artificial concurrency
		// limit.
		Parallelism: instanceCount * 3, // instanceCount instances of 2 resources, plus an excessive amount of headroom for other helper nodes
	})
	ctx := context.Background()
	priorStateBase := states.BuildState(func(ss *states.SyncState) {
		// Our prior state already has all of the instances declared in
		// the configuration, so that we can also exercise the "upgrade"
		// and "refresh" steps (which are no-op in the mock provider we're
		// using, so we're only measuring their overhead).
		providerAddr := addrs.AbsProviderConfig{
			Module:   addrs.RootModule,
			Provider: addrs.NewBuiltInProvider("test"),
		}
		resourceAddrA := addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "test",
			Name: "a",
		}.Absolute(addrs.RootModuleInstance)
		resourceAddrB := addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "test",
			Name: "b",
		}.Absolute(addrs.RootModuleInstance)
		for i := range instanceCount {
			instAddrA := resourceAddrA.Instance(addrs.IntKey(i))
			instAddrB := resourceAddrB.Instance(addrs.IntKey(i))
			rawStateAttrs := `{"num":` + strconv.Itoa(i) + `}`
			ss.SetResourceInstanceCurrent(
				instAddrA,
				&states.ResourceInstanceObjectSrc{
					AttrsJSON: []byte(rawStateAttrs),
				},
				providerAddr, addrs.NoKey,
			)
			ss.SetResourceInstanceCurrent(
				instAddrB,
				&states.ResourceInstanceObjectSrc{
					AttrsJSON: []byte(rawStateAttrs),
				},
				providerAddr, addrs.NoKey,
			)
		}
	})
	planOpts := &PlanOpts{
		Mode: plans.NormalMode,
		SetVariables: InputValues{
			"instance_count": {
				Value: cty.NumberIntVal(instanceCount),
			},
		},
	}
	b.ResetTimer() // the above setup code is not included in the benchmark

	for range b.N {
		// It's unfortunate to include this as part of the benchmark, but
		// our work below is going to modify the state in-place so we do need
		// to copy it. In practice the CLI layer's state manager system will
		// tend to do at least one state DeepCopy as part of setting itself up
		// anyway, so this is not unrealistic.
		priorState := priorStateBase.DeepCopy()

		plan, planDiags := tofuCtx.Plan(ctx, m, priorState, planOpts)
		assertNoDiagnostics(b, planDiags)

		_, applyDiags := tofuCtx.Apply(ctx, plan, m, nil)
		assertNoDiagnostics(b, applyDiags)
	}
}

// This benchmark takes, at the time of writing, several seconds per iteration, and
// so it's probably best to limit the amount of time it can run:
//
//	go test ./internal/tofu -bench='^BenchmarkManyModuleInstances$' -benchtime=1m
func BenchmarkManyModuleInstances(b *testing.B) {
	// instanceCount is the number of instances we declare for each module call.
	// Since there are two module calls, each object declared in the module
	// is instantiated twice per instanceCount.
	const instanceCount = 2500
	m := testModuleInline(b, map[string]string{
		"main.tf": `
			variable "instance_count" {
				type = number
			}

			module "a" {
				source = "./child"
				count  = var.instance_count

				num = count.index
			}

			module "b" {
				source = "./child"
				count  = length(module.a)

				num = module.a[count.index].num
			}
		`,
		"child/child.tf": `
			variable "num" {
				type = number
			}

			# Intentionally no resources declared here, because this
			# test is measuring just the module call overhead and
			# administrative overhead like the input variable and
			# output value evaluation.

			output "num" {
				value = var.num
			}
		`,
	})
	tofuCtx := testContext2(b, &ContextOpts{
		Providers: nil, // no providers for this test
		// With this many resource instances we need a high concurrency limit
		// for the runtime to be in any way reasonable. In this case we're
		// going to set it so high that there is effectively no limit at all,
		// which measures a best-case scenario where we're limited only by
		// OpenTofu's direct overheads and not by the artificial concurrency
		// limit.
		Parallelism: instanceCount * 2 * 8, // instanceCount instances of 2 modules, with enough headroom for 8 graph nodes each (intentionally more than needed)
	})
	ctx := context.Background()
	planOpts := &PlanOpts{
		Mode: plans.NormalMode,
		SetVariables: InputValues{
			"instance_count": {
				Value: cty.NumberIntVal(instanceCount),
			},
		},
	}
	b.ResetTimer() // the above setup code is not included in the benchmark

	for range b.N {
		plan, planDiags := tofuCtx.Plan(ctx, m, states.NewState(), planOpts)
		assertNoDiagnostics(b, planDiags)

		_, applyDiags := tofuCtx.Apply(ctx, plan, m, nil)
		assertNoDiagnostics(b, applyDiags)
	}
}
