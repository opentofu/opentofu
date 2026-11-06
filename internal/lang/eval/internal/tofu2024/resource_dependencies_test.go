// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/providers"
)

func TestResourceDependencyAnalysis(t *testing.T) {
	tests := []struct {
		IndexVal cty.Value
		WantDeps map[string]struct{}
	}{
		{
			IndexVal: cty.UnknownVal(cty.String),
			WantDeps: map[string]struct{}{
				"test.a": struct{}{},
				"test.b": struct{}{},
			},
		},
		{
			IndexVal: cty.DynamicVal,
			WantDeps: map[string]struct{}{
				"test.a": struct{}{},
				"test.b": struct{}{},
			},
		},
		{
			IndexVal: cty.StringVal("a"),
			WantDeps: map[string]struct{}{
				"test.a": struct{}{},
			},
		},
		{
			IndexVal: cty.StringVal("b"),
			WantDeps: map[string]struct{}{
				"test.b": struct{}{},
			},
		},
		{
			IndexVal: cty.UnknownVal(cty.String).Refine().
				NotNull().
				StringPrefixFull("a").
				NewValue(),
			WantDeps: map[string]struct{}{
				"test.a": struct{}{},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.IndexVal.GoString(), func(t *testing.T) {
			mod := ModuleFromSourceForTesting(t, `
			terraform {
				required_providers {
					test = {
						source = "terraform.io/builtin/test"
					}
				}
			}

			variable "index" {
				type = string
			}

			resource "test" "a" {
			}
			resource "test" "b" {
			}

			locals {
				objs = tomap({
					a = {
						id = test.a.id
					}
					b = {
						id = test.b.id
					}
				})
			}

			output "obj" {
				# var.index is unknown, so this is an unknown choice between
				# elements a and b.
				value = local.objs[var.index]
			}
		`)
			ctx := grapheval.ContextWithNewWorker(t.Context())
			ctx = grapheval.ContextWithRequestTracker(ctx, grapheval.NewRequestTracker())
			inst := CompileModuleInstance(
				ctx,
				mod,
				addrs.ModuleSourceLocal("."),
				&ModuleInstanceCall{
					CalleeAddr: addrs.RootModuleInstance,
					InputValues: exprs.ConstantValuer(cty.ObjectVal(map[string]cty.Value{
						// intentionally unknown so we can make sure the
						// dependency marks from both "a" and "b" get propagated
						// when we don't know which element we're selecting.
						"index": test.IndexVal,
					})),
					EvaluationGlue: &evalGlueForTesting{},
					EvalContext: &evalglue.EvalContext{
						RootModuleDir: ".",
						Providers: evalglue.ProvidersForTesting(map[addrs.Provider]*providers.GetProviderSchemaResponse{
							addrs.NewBuiltInProvider("test"): &providers.GetProviderSchemaResponse{},
						}),
					},
				},
			)
			// We're going straight to the result valuer here, which means we'll
			// evaluate the output value but intentionally ignore any errors
			// from elsewhere in the configuration. The only thing we care about
			// here is that the output value ends up marked as depending on both
			// of the resource instances, since we don't yet know which one would
			// be selected once the index is known.
			resultValuer := inst.ResultValuer(ctx)
			got, diags := resultValuer.Value(ctx)
			if diags.HasErrors() {
				t.Errorf("unexpected errors: %s", diags.Err().Error())
			}
			gotDeps := make(map[string]struct{})
			for inst := range configgraph.ContributingResourceInstances(got) {
				gotDeps[inst.Addr.String()] = struct{}{}
			}
			wantDeps := test.WantDeps
			if diff := cmp.Diff(wantDeps, gotDeps); diff != "" {
				t.Error("wrong deps\n" + diff)
			}
		})
	}
}
