// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

func TestTypeConversionConstraint(t *testing.T) {
	tests := map[string]struct {
		InputValue     cty.Value
		ConstraintExpr string

		WantValue cty.Value
		WantError string
	}{
		// The following tests are intentionally focused mainly on verifying
		// that [TypeConversionConstraint.ConvertValue] is performing its
		// steps in the correct order and propagating values out. It is not
		// attempting to exhaustively test all of the internal behaviors of
		// the two functions that ConvertValue is implemented in terms of,
		// although a more exhaustive testing strategy may become appropriate
		// if a future version of this function stops just being a thin
		// wrapper around two upstream functions and starts containing real
		// logic of its own that isn't already tested elsewhere.
		"string to any": {
			InputValue:     cty.StringVal("hello"),
			ConstraintExpr: `any`,
			WantValue:      cty.StringVal("hello"),
		},
		"string to number": {
			InputValue:     cty.StringVal("10"),
			ConstraintExpr: `number`,
			WantValue:      cty.NumberIntVal(10),
		},
		"string to number invalid": {
			InputValue:     cty.StringVal("hello"),
			ConstraintExpr: `number`,
			WantError:      `a number is required`,
		},

		"object type with no optional attributes": {
			InputValue:     cty.ObjectVal(map[string]cty.Value{"name": cty.StringVal("Treguard")}),
			ConstraintExpr: `object({ name = string })`,
			WantValue:      cty.ObjectVal(map[string]cty.Value{"name": cty.StringVal("Treguard")}),
		},
		"object type with an optional attribute that has no default": {
			InputValue:     cty.EmptyObjectVal,
			ConstraintExpr: `object({ name = optional(string) })`,
			WantValue:      cty.ObjectVal(map[string]cty.Value{"name": cty.NullVal(cty.String)}),
		},
		"object type with an optional attribute that has a default": {
			InputValue:     cty.EmptyObjectVal,
			ConstraintExpr: `object({ name = optional(string, "Pickle") })`,
			WantValue:      cty.ObjectVal(map[string]cty.Value{"name": cty.StringVal("Pickle")}),
		},
		"object type discarding attribute during conversion": {
			InputValue:     cty.ObjectVal(map[string]cty.Value{"name": cty.StringVal("Majida")}),
			ConstraintExpr: `object({})`,
			WantValue:      cty.EmptyObjectVal,
		},
		"object type with an optional attribute that has a default for a null value": {
			InputValue:     cty.NullVal(cty.EmptyObject),
			ConstraintExpr: `object({ name = optional(string, "Motley") })`,
			WantValue:      cty.NullVal(cty.Object(map[string]cty.Type{"name": cty.String})),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			expr, hclDiags := hclsyntax.ParseExpression([]byte(test.ConstraintExpr), "", hcl.InitialPos)
			if hclDiags.HasErrors() {
				t.Fatal(hclDiags.Error())
			}
			constraint, diags := ParseTypeConversionConstraint(expr)
			if diags.HasErrors() {
				// We're intentionally not testing parsing errors here because
				// our current implementation is just a thing wrapper around
				// a function in upstream HCL, and that function already has
				// tests in HCL. If a future version of this function starts
				// having additional custom behavior beyond what's in upstream
				// HCL then we should probably split this test into two parts
				// and test parsing separately from conversion.
				t.Fatal(diags.Err().Error())
			}

			gotVal, gotErr := constraint.ConvertValue(test.InputValue)

			if wantErr := test.WantError; wantErr != "" {
				if gotErr == nil {
					t.Fatalf("unexpected success\nwant error: %s", wantErr)
				}
				if gotErr := gotErr.Error(); gotErr != wantErr {
					t.Fatalf("wrong error\ngot:  %s\nwant: %s", gotErr, wantErr)
				}
				return
			}

			if gotErr != nil {
				t.Fatalf("unexpected error\ngot error: %s\nwant result: %#v", gotErr.Error(), test.WantValue)
			}
			if wantVal := test.WantValue; !wantVal.RawEquals(gotVal) {
				t.Fatalf("wrong result\ngot:  %#v\nwant: %#v", gotVal, wantVal)
			}
		})
	}
}

func TestConvertFunc(t *testing.T) {
	tests := []struct {
		CallExpr  string
		WantValue cty.Value
		WantError string
	}{
		{
			CallExpr:  `convert("hello", string)`,
			WantValue: cty.StringVal("hello"),
		},
		{
			CallExpr:  `convert("true", bool)`,
			WantValue: cty.True,
		},
		{
			CallExpr:  `convert("hello", bool)`,
			WantError: `:1,10-15: Invalid function argument; Invalid value for "value" parameter: a bool is required.`,
		},
		{
			CallExpr:  `convert(marked, string)`,
			WantValue: cty.StringVal("marked").Mark("mark"),
		},
		{
			CallExpr: `convert({}, object({name = optional(string)}))`,
			WantValue: cty.ObjectVal(map[string]cty.Value{
				"name": cty.NullVal(cty.String),
			}),
		},
		{
			CallExpr: `convert({}, object({name = optional(string, "Jackson")}))`,
			WantValue: cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("Jackson"),
			}),
		},
		{
			CallExpr:  `convert(null, object({name = optional(string, "Jackson")}))`,
			WantValue: cty.NullVal(cty.Object(map[string]cty.Type{"name": cty.String})),
		},
		{
			CallExpr:  `convert(unknown, object({name = optional(string, "Jackson")}))`,
			WantValue: cty.UnknownVal(cty.Object(map[string]cty.Type{"name": cty.String})),
		},
		{
			CallExpr:  `convert({name = marked}, object({name = string}))`,
			WantValue: cty.ObjectVal(map[string]cty.Value{"name": cty.StringVal("marked").Mark("mark")}),
		},
	}

	convertFunc := makeConvertFunc()
	hclCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"unknown": cty.DynamicVal,
			"marked":  cty.StringVal("marked").Mark("mark"),
		},
		Functions: map[string]function.Function{
			"convert": convertFunc,
		},
	}

	for _, test := range tests {
		t.Run(test.CallExpr, func(t *testing.T) {
			expr, hclDiags := hclsyntax.ParseExpression([]byte(test.CallExpr), "", hcl.InitialPos)
			if hclDiags.HasErrors() {
				t.Fatal(hclDiags.Error())
			}

			gotVal, gotDiags := expr.Value(hclCtx)

			if test.WantError != "" {
				if !gotDiags.HasErrors() {
					t.Fatalf("unexpected success\nwant error: %s", test.WantError)
				}
				if gotErr, wantErr := gotDiags.Error(), test.WantError; gotErr != wantErr {
					t.Fatalf("wrong error\ngot:  %s\nwant: %s", gotErr, wantErr)
				}
				return
			}

			if gotDiags.HasErrors() {
				t.Fatalf("unexpected error\ngot error: %s\nwant value: %#v", gotDiags.Error(), test.WantValue)
			}
			if wantVal := test.WantValue; !wantVal.RawEquals(gotVal) {
				t.Fatalf("wrong result\ngot:  %#v\nwant: %#v", gotVal, wantVal)
			}
		})
	}
}
