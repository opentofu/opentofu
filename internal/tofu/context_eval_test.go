// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/zclconf/go-cty/cty"
)

func TestContextEval(t *testing.T) {
	// This test doesn't check the "Want" value for impure funcs, so the value
	// on those doesn't matter.
	tests := []struct {
		Input      string
		Want       cty.Value
		ImpureFunc bool
	}{
		{ // An impure function: allowed in the console, but the result is nondeterministic
			`bcrypt("example")`,
			cty.NilVal,
			true,
		},
		{
			`keys(var.map)`,
			cty.ListVal([]cty.Value{
				cty.StringVal("foo"),
				cty.StringVal("baz"),
			}),
			true,
		},
		{
			`local.result`,
			cty.NumberIntVal(6),
			false,
		},
		{
			`module.child.result`,
			cty.NumberIntVal(6),
			false,
		},
	}

	// This module has a little bit of everything (and if it is missing something, add to it):
	// resources, variables, locals, modules, output
	m := testModule(t, "eval-context-basic")
	p := testProvider("test")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	scope, diags := ctx.Eval(context.Background(), m, states.NewState(), addrs.RootModuleInstance, &EvalOpts{
		SetVariables: testInputValuesUnset(m.Module.Variables),
	})
	if diags.HasErrors() {
		t.Fatalf("Eval errors: %s", diags.Err())
	}

	// Since we're testing 'eval' (used by tofu console), impure functions
	// should be allowed by the scope.
	if scope.PureOnly == true {
		t.Fatal("wrong result: eval should allow impure funcs")
	}

	for _, test := range tests {
		t.Run(test.Input, func(t *testing.T) {
			// Parse the test input as an expression
			expr, _ := hclsyntax.ParseExpression([]byte(test.Input), "<test-input>", hcl.Pos{Line: 1, Column: 1})
			got, diags := scope.EvalExpr(t.Context(), expr, cty.DynamicPseudoType)

			if diags.HasErrors() {
				t.Fatalf("unexpected error: %s", diags.Err())
			}

			if !test.ImpureFunc {
				if !got.RawEquals(test.Want) {
					t.Fatalf("wrong result: want %#v, got %#v", test.Want, got)
				}
			}
		})
	}
}

// ensure that we can execute a console when outputs have preconditions
func TestContextEval_outputsWithPreconditions(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
module "mod" {
  source = "./mod"
  input  = "ok"
}

output "out" {
  value = module.mod.out
}
`,

		"./mod/main.tf": `
variable "input" {
  type = string
}

output "out" {
  value = var.input

  precondition {
    condition     = var.input != ""
    error_message = "error"
  }
}
`,
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Eval(context.Background(), m, states.NewState(), addrs.RootModuleInstance, &EvalOpts{
		SetVariables: testInputValuesUnset(m.Module.Variables),
	})
	assertNoErrors(t, diags)
}
