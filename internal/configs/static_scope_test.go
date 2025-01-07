// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"
)

func TestStaticScope_GetInputVariable(t *testing.T) {
	test_ident := StaticIdentifier{
		Subject: "local.test_scenario",
	}
	t.Run("valid", func(t *testing.T) {
		// This covers an assortment of valid cases that we can test in a single
		// pass because they are all independent of one another.
		p := testParser(map[string]string{
			"test.tf": `
				variable "unconstrained" {
				}

				variable "string" {
					type = string
				}

				variable "string_from_number" {
					type = string
				}

				variable "list_from_tuple" {
					type = list(string)
				}

				variable "set_from_tuple" {
					type = set(string)
				}

				variable "map_from_object" {
					type = map(string)
				}

				variable "non_nullable_default" {
					type     = string
					default  = "default value"
					nullable = false
				}

				# NOTE: We're not currently testing the replacement of a
				# completely-unset variable by its default here because
				# that's currently the responsibility of each separate
				# source of input variables included in a StaticModuleCall,
				# rather than part of the StaticScope functionality.

				variable "object_with_optional" {
					type = object({
						a = optional(string)
						b = optional(string, "b default")
						c = string
					})
				}

				variable "nullable_null_string" {
					type    = string
					default = "..." # Ignored because nullable and set to null in the tests below
				}

				variable "unknown_string" {
					type = string
				}

				variable "sensitive_string" {
					type      = string
					sensitive = true
				}
			`,
		})

		tests := map[string]struct {
			callerVal    cty.Value
			wantFinalVal cty.Value
		}{
			"unconstrained": {
				cty.EmptyObjectVal,
				cty.EmptyObjectVal,
			},
			"string": {
				cty.StringVal("hello"),
				cty.StringVal("hello"),
			},
			"string_from_number": {
				cty.NumberIntVal(12),
				cty.StringVal("12"),
			},
			"list_from_tuple": {
				cty.TupleVal([]cty.Value{
					cty.StringVal("a"),
					cty.True,
					cty.StringVal("a"),
				}),
				cty.ListVal([]cty.Value{
					cty.StringVal("a"),
					cty.StringVal("true"),
					cty.StringVal("a"),
				}),
			},
			"set_from_tuple": {
				cty.TupleVal([]cty.Value{
					cty.StringVal("a"),
					cty.True,
					cty.StringVal("a"),
				}),
				cty.SetVal([]cty.Value{
					cty.StringVal("a"), // coalesced
					cty.StringVal("true"),
				}),
			},
			"map_from_object": {
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.StringVal("a value"),
					"b": cty.False,
				}),
				cty.MapVal(map[string]cty.Value{
					"a": cty.StringVal("a value"),
					"b": cty.StringVal("false"),
				}),
			},
			"non_nullable_default": {
				cty.NullVal(cty.String),
				cty.StringVal("default value"), // null replaced by default because nullable = false
			},
			"object_with_optional": {
				cty.ObjectVal(map[string]cty.Value{
					"c": cty.StringVal("c value"),
					"d": cty.StringVal("d value"),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.NullVal(cty.String),
					"b": cty.StringVal("b default"),
					"c": cty.StringVal("c value"),
					// d dropped because it's not part of the type constraint
				}),
			},
			"nullable_null_string": {
				// This case is a historical design error that was later partially fixed by allowing
				// nullable = false, but nullable is true by default for backward-compatibility.
				cty.NullVal(cty.String),
				cty.NullVal(cty.String), // nullable = true by default, so explicit null overrides default
			},
			"unknown_string": {
				cty.UnknownVal(cty.String),
				cty.UnknownVal(cty.String),
			},
			"sensitive_string": {
				cty.StringVal("ssshhhhh it's a secret"),
				cty.StringVal("ssshhhhh it's a secret").Mark(marks.Sensitive),
			},
		}

		call := NewStaticModuleCall(
			addrs.RootModule,
			func(v *Variable) (cty.Value, hcl.Diagnostics) {
				return tests[v.Name].callerVal, nil
			},
			".",
			"irrelevant",
		)
		mod, diags := p.LoadConfigDir(".", call)
		assertNoDiagnostics(t, diags)

		// We'll make sure the config and the test cases remain consistent
		// under future maintenence. A failure here suggests that the test
		// is wrong, rather than the code under test.
		for name := range mod.Variables {
			if _, exists := tests[name]; !exists {
				t.Errorf("no test case for declared variable %q", name)
			}
		}
		for name := range tests {
			if _, exists := mod.Variables[name]; !exists {
				t.Errorf("no declared variable for test case %q", name)
			}
		}

		eval := NewStaticEvaluator(mod, call)
		scope := newStaticScope(eval, test_ident)

		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				addr := addrs.InputVariable{Name: name}
				gotFinalVal, moreDiags := scope.Data.GetInputVariable(addr, tfdiags.SourceRange{Filename: "test.tf"})
				assertNoDiagnostics(t, moreDiags.ToHCL())

				if !test.wantFinalVal.RawEquals(gotFinalVal) {
					diff := cmp.Diff(test.wantFinalVal, gotFinalVal, ctydebug.CmpOptions)
					t.Errorf("wrong result\ninput: %#v for %s\ngot:   %#v\nwant:  %#v\n\n%s", test.callerVal, addr, gotFinalVal, test.wantFinalVal, diff)
				}
			})
		}
	})
	t.Run("incompatible type", func(t *testing.T) {
		p := testParser(map[string]string{
			"test.tf": `
				variable "bad_default" {
					type = list(string)
				}
			`,
		})

		call := NewStaticModuleCall(
			addrs.RootModule,
			func(v *Variable) (cty.Value, hcl.Diagnostics) {
				return cty.StringVal("not a list"), nil
			},
			".",
			"irrelevant",
		)
		mod, diags := p.LoadConfigDir(".", call)
		assertNoDiagnostics(t, diags)

		eval := NewStaticEvaluator(mod, call)
		scope := newStaticScope(eval, test_ident)

		addr := addrs.InputVariable{Name: "bad_default"}
		_, moreDiags := scope.Data.GetInputVariable(addr, tfdiags.SourceRange{Filename: "test.tf"})
		if !moreDiags.HasErrors() {
			t.Fatal("unexpected success; want a type conversion error")
		}
		diagsStr := moreDiags.Err().Error()
		// The part of the message we're matching on below is actually generated by upstream
		// library cty, so if a future cty upgrade changes this message to something substantially
		// similar then it's fine to change this test to match but the message should always
		// be about the given value being of the wrong type.
		if want := "list of string required"; !strings.Contains(diagsStr, want) {
			t.Errorf("wrong error\ngot: %s\nwant message containing: %s", diagsStr, want)
		}
	})
	t.Run("non-nullable null", func(t *testing.T) {
		p := testParser(map[string]string{
			"test.tf": `
				variable "not_nullable" {
					type     = string
					nullable = false
				}
			`,
		})

		call := NewStaticModuleCall(
			addrs.RootModule,
			func(v *Variable) (cty.Value, hcl.Diagnostics) {
				return cty.NullVal(cty.String), nil
			},
			".",
			"irrelevant",
		)
		mod, diags := p.LoadConfigDir(".", call)
		assertNoDiagnostics(t, diags)

		eval := NewStaticEvaluator(mod, call)
		scope := newStaticScope(eval, test_ident)

		addr := addrs.InputVariable{Name: "not_nullable"}
		_, moreDiags := scope.Data.GetInputVariable(addr, tfdiags.SourceRange{Filename: "test.tf"})
		if !moreDiags.HasErrors() {
			t.Fatal("unexpected success; want an error about the variable being required")
		}
		diagsStr := moreDiags.Err().Error()
		if want := "required variable may not be set to null"; !strings.Contains(diagsStr, want) {
			t.Errorf("wrong error\ngot: %s\nwant message containing: %s", diagsStr, want)
		}
	})
}

func TestStaticScope_GetLocalValue(t *testing.T) {
	test_ident := StaticIdentifier{
		Subject: "local.test_scenario",
	}

	t.Run("valid", func(t *testing.T) {
		p := testParser(map[string]string{
			"test.tf": `
				locals {
					foo = "bar"
				}
			`,
		})

		call := NewStaticModuleCall(
			addrs.RootModule,
			func(v *Variable) (cty.Value, hcl.Diagnostics) {
				var diags tfdiags.Diagnostics
				diags = diags.Append(fmt.Errorf("no variables here"))
				return cty.DynamicVal, diags.ToHCL()
			},
			".",
			"irrelevant",
		)
		mod, diags := p.LoadConfigDir(".", call)
		assertNoDiagnostics(t, diags)

		eval := NewStaticEvaluator(mod, call)
		scope := newStaticScope(eval, test_ident)

		addr := addrs.LocalValue{Name: "foo"}
		got, moreDiags := scope.Data.GetLocalValue(addr, tfdiags.SourceRange{Filename: "test.tf"})
		want := cty.StringVal("bar")
		assertNoDiagnostics(t, moreDiags.ToHCL())
		if !got.RawEquals(want) {
			t.Errorf("wrong result\ngot:  %#v\nwant: %#v", got, want)
		}
	})
	t.Run("undeclared", func(t *testing.T) {
		p := testParser(map[string]string{
			"test.tf": `
				# This module intentionally left blank
			`,
		})

		call := NewStaticModuleCall(
			addrs.RootModule,
			func(v *Variable) (cty.Value, hcl.Diagnostics) {
				var diags tfdiags.Diagnostics
				diags = diags.Append(fmt.Errorf("no variables here"))
				return cty.DynamicVal, diags.ToHCL()
			},
			".",
			"irrelevant",
		)
		mod, diags := p.LoadConfigDir(".", call)
		assertNoDiagnostics(t, diags)

		eval := NewStaticEvaluator(mod, call)
		scope := newStaticScope(eval, test_ident)

		addr := addrs.LocalValue{Name: "nonexist"}
		_, moreDiags := scope.Data.GetLocalValue(addr, tfdiags.SourceRange{Filename: "test.tf"})
		if !moreDiags.HasErrors() {
			t.Fatal("unexpected success; want error")
		}
		diagsStr := moreDiags.Err().Error()
		if want := "Undefined local local.nonexist"; !strings.Contains(diagsStr, want) {
			t.Errorf("wrong error\ngot: %s\nwant message containing: %s", diagsStr, want)
		}
	})
}
