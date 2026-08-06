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
