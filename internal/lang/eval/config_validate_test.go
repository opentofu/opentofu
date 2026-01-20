// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval_test

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// This file is in "package eval_test" in order to integration-test the
// validation phase through the same exported API that external callers would
// use.

func TestValidate_valuesOnlySuccess(t *testing.T) {
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

	diags = configInst.Validate(t.Context())
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}
}

func TestValidate_valuesOnlyError(t *testing.T) {
	// This test has an intentionally limited scope covering just the
	// basics, so that we don't necessarily need to repeat these basics
	// across all of the other tests.
	configInst, diags := eval.NewConfigInstance(t.Context(), &eval.ConfigCall{
		EvalContext: evalglue.EvalContextForTesting(t, &eval.EvalContext{
			Modules: eval.ModulesForTesting(map[addrs.ModuleSourceLocal]*configs.Module{
				addrs.ModuleSourceLocal("."): configs.ModuleFromStringForTesting(t, `
					variable "a" {
						type = any
					}
					locals {
						b = "${var.a}!"
					}
					output "c" {
						value = "${local.b}!"
					}
				`),
			}),
		}),
		RootModuleSource: addrs.ModuleSourceLocal("."),
		InputValues: eval.InputValuesForTesting(map[string]cty.Value{
			"a": cty.EmptyObjectVal, // not valid for how the variable is used elsewhere in the module
		}),
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	var wantDiags tfdiags.Diagnostics
	wantDiags = wantDiags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		// If a future HCL upgrade changes the presentation of this error then
		// it's okay to update this to match as long as the new error is still
		// about using an object where a string is expected, and still gets
		// reported in the template interpolation for local value "b".
		Summary: "Invalid template interpolation value",
		Detail:  `Cannot include the given value in a string template: string required, but have object.`,
		Subject: &hcl.Range{ // the var.a reference
			Filename: "<ModuleFromStringForTesting>",
			Start:    hcl.Pos{Line: 6, Column: 14, Byte: 72},
			End:      hcl.Pos{Line: 6, Column: 19, Byte: 77},
		},
		Context: &hcl.Range{ // the entire ${var.a} interpolation sequence
			Filename: "<ModuleFromStringForTesting>",
			Start:    hcl.Pos{Line: 6, Column: 11, Byte: 69},
			End:      hcl.Pos{Line: 6, Column: 22, Byte: 80},
		},
	})
	gotDiags := configInst.Validate(t.Context())
	assertDiagnosticsMatch(t, gotDiags, wantDiags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}
}

func TestValidate_valuesOnlyCycle(t *testing.T) {
	// This test has an intentionally limited scope covering just the
	// basics, so that we don't necessarily need to repeat these basics
	// across all of the other tests.
	configInst, diags := eval.NewConfigInstance(t.Context(), &eval.ConfigCall{
		EvalContext: evalglue.EvalContextForTesting(t, &eval.EvalContext{
			Modules: eval.ModulesForTesting(map[addrs.ModuleSourceLocal]*configs.Module{
				addrs.ModuleSourceLocal("."): configs.ModuleFromStringForTesting(t, `
					locals {
						a = local.b
						b = local.a
					}
				`),
			}),
		}),
		RootModuleSource: addrs.ModuleSourceLocal("."),
		InputValues:      eval.InputValuesForTesting(map[string]cty.Value{}),
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	// The self-reference detection causes all objects involved in the cycle
	// to fail at once, so we get diagnostics for both a and b here.
	// TODO: Consider adding ExtraInfo to these diagnostics to mark them
	// as self-reference related and then coalescing them either in this
	// package or at the UI layer. Doing it at the CLI layer would allow it
	// to be predictable which one it selects without us needing to redundantly
	// presort the diagnostics here; sorting and coalescing is normally the
	// CLI layer's concern.
	var wantDiags tfdiags.Diagnostics
	wantDiags = wantDiags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		// If a future HCL upgrade changes the presentation of this error then
		// it's okay to update this to match as long as the new error is still
		// about using an object where a string is expected, and still gets
		// reported in the template interpolation for local value "b".
		Summary: "Self-referential expressions",
		Detail: `The following objects in the configuration form a dependency cycle, so there is no valid order to evaluate them in:
  - local.a (<ModuleFromStringForTesting>:3,11)
  - local.b (<ModuleFromStringForTesting>:4,11)`,
	})
	wantDiags = wantDiags.Append(wantDiags[0])
	gotDiags := configInst.Validate(t.Context())
	gotDiags.Sort() // we don't care what order they are in
	assertDiagnosticsMatch(t, gotDiags, wantDiags)
}

func TestValidate_resourceValid(t *testing.T) {
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
					variable "in" {
						type = string
					}
					resource "foo" "bar" {
						name = var.in
					}
					output "out" {
						value = foo.bar.id
					}
				`),
			}),
			Providers: eval.ProvidersForTesting(map[addrs.Provider]*providers.GetProviderSchemaResponse{
				addrs.MustParseProviderSourceString("test/foo"): {
					Provider: providers.Schema{
						Block: &configschema.Block{},
					},
					ResourceTypes: map[string]providers.Schema{
						"foo": {
							Block: &configschema.Block{
								Attributes: map[string]*configschema.Attribute{
									"name": {
										Type:     cty.String,
										Required: true,
									},
									"id": {
										Type:     cty.String,
										Computed: true,
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
			"in": cty.StringVal("foo bar baz"),
		}),
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	diags = configInst.Validate(t.Context())
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}
}

func TestValidate_childModuleCallValuesOnly(t *testing.T) {
	configInst, diags := eval.NewConfigInstance(t.Context(), &eval.ConfigCall{
		EvalContext: evalglue.EvalContextForTesting(t, &eval.EvalContext{
			Modules: eval.ModulesForTesting(map[addrs.ModuleSourceLocal]*configs.Module{
				addrs.ModuleSourceLocal("."): configs.ModuleFromStringForTesting(t, `
					variable "in" {
						type = string
					}
					module "child" {
						source = "./child"

						input = var.in
					}
					output "out" {
						value = module.child.result
					}
				`),
				addrs.ModuleSourceLocal("./child"): configs.ModuleFromStringForTesting(t, `
					variable "input" {
						type = string
					}
					output "result" {
						value = var.input
					}
				`),
			}),
			Providers: eval.ProvidersForTesting(map[addrs.Provider]*providers.GetProviderSchemaResponse{
				addrs.MustParseProviderSourceString("test/foo"): {
					Provider: providers.Schema{
						Block: &configschema.Block{},
					},
					ResourceTypes: map[string]providers.Schema{
						"foo": {
							Block: &configschema.Block{
								Attributes: map[string]*configschema.Attribute{
									"name": {
										Type:     cty.String,
										Required: true,
									},
									"id": {
										Type:     cty.String,
										Computed: true,
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
			"in": cty.StringVal("foo bar baz"),
		}),
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	diags = configInst.Validate(t.Context())
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}
}
