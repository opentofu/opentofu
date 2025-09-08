// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval_test

import (
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/providers"
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
// -------------------------------------------------------------------------

// This is an adaptation of a benchmark test of the same name in "package tofu"
// for comparison purposes.
//
// However, it's not yet really a fair comparison because the original form
// of this test covered a full plan/apply round whereas this is currently
// only performing validation because at the time of writing the plan/apply
// entry points are not written yet.
//
// However, "validate" in this package actually does instance expansion while
// in "package tofu" it only did static checks of the declarations in each
// module, so this is a _slightly_ more relevant comparison than it might
// initially appear to be. It still saves a lot of time not even trying to
// upgrade, refresh, or plan anything from prior state though. (For this to
// be a really fair comparison it will likely need to end up in a different
// package where it can also run the real planning machinery rather than
// mocking it, because this package does not have the planning engine in its
// scope.)
//
//	go test ./internal/lang/eval -bench='^BenchmarkManyResourceInstances$'
func BenchmarkManyResourceInstances(b *testing.B) {
	// instanceCount is the number of instances we declare _for each resource_.
	// Since there are two resources, there are 2*instanceCount instances total.
	const instanceCount = 2500
	configInst, diags := eval.NewConfigInstance(b.Context(), &eval.ConfigCall{
		EvalContext: evalglue.EvalContextForTesting(b, &eval.EvalContext{
			Modules: eval.ModulesForTesting(map[addrs.ModuleSourceLocal]*configs.Module{
				addrs.ModuleSourceLocal("."): configs.ModuleFromStringForTesting(b, `
					# This test has two resources that each have a lot of instances
					# that are correlated with one another.

					terraform {
						required_providers {
							foo = {
								source = "test/foo"
							}
						}
					}

					variable "instance_count" {
						type = number
					}

					resource "foo" "a" {
						count = var.instance_count

						num = count.index
					}

					resource "foo" "b" {
						count = var.instance_count

						num = foo.a[count.index].num
					}
				`),
			}),
			Providers: eval.ProvidersForTesting(map[addrs.Provider]*providers.GetProviderSchemaResponse{
				addrs.MustParseProviderSourceString("test/foo"): {
					ResourceTypes: map[string]providers.Schema{
						"foo": {
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
			}),
		}),
		RootModuleSource: addrs.ModuleSourceLocal("."),
		InputValues: eval.InputValuesForTesting(map[string]cty.Value{
			"instance_count": cty.NumberIntVal(instanceCount),
		}),
	})
	if diags.HasErrors() {
		b.Fatalf("unexpected errors: %s", diags.Err())
	}

	b.ResetTimer() // the above setup code is not included in the benchmark

	for b.Loop() {
		diags = configInst.Validate(b.Context())
		if diags.HasErrors() {
			b.Fatalf("unexpected errors: %s", diags.Err())
		}
	}
}
