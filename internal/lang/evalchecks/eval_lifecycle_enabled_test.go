// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package evalchecks

import (
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

func TestEvaluateEnabledExpression_valid(t *testing.T) {
	tests := map[string]struct {
		expr     hcl.Expression
		expected bool
	}{
		"true": {
			hcltest.MockExprLiteral(cty.BoolVal(true)),
			true,
		},
		"equal condition": {
			&hclsyntax.BinaryOpExpr{
				LHS: &hclsyntax.LiteralValueExpr{
					Val: cty.StringVal("5"),
				},
				RHS: &hclsyntax.LiteralValueExpr{
					Val: cty.StringVal("5"),
				},
				Op: hclsyntax.OpEqual,
			},
			true,
		},
		"unequal condition": {
			&hclsyntax.BinaryOpExpr{
				LHS: &hclsyntax.LiteralValueExpr{
					Val: cty.StringVal("3"),
				},
				RHS: &hclsyntax.LiteralValueExpr{
					Val: cty.StringVal("5"),
				},
				Op: hclsyntax.OpEqual,
			},
			false,
		},
		"false": {
			hcltest.MockExprLiteral(cty.BoolVal(false)),
			false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual, diags := EvaluateEnabledExpression(test.expr, mockRefsFunc())

			if len(diags) != 0 {
				t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
			}

			if !reflect.DeepEqual(actual, test.expected) {
				t.Errorf(
					"wrong map value\ngot:  %swant: %s",
					spew.Sdump(actual), spew.Sdump(test.expected),
				)
			}
		})
	}
}

type WantedError struct {
	Summary         string
	DetailSubstring string
}

func TestEvaluateEnabledExpression_errors(t *testing.T) {
	tests := map[string]struct {
		expr   hcl.Expression
		Wanted []WantedError
	}{
		"null": {
			hcltest.MockExprLiteral(cty.NullVal(cty.Number)),
			[]WantedError{
				{
					"Invalid enabled argument",
					`The given "enabled" argument value is unsuitable: the given value is null.`,
				},
			},
		},
		"positive number": {
			hcltest.MockExprLiteral(cty.NumberIntVal(1)),
			[]WantedError{
				{
					"Invalid enabled argument",
					`The given "enabled" argument value is unsuitable: bool required, but have number.`,
				},
			},
		},
		"negative number": {
			hcltest.MockExprLiteral(cty.NumberIntVal(-1)),
			[]WantedError{
				{
					"Invalid enabled argument",
					`The given "enabled" argument value is unsuitable: bool required, but have number.`,
				},
			},
		},
		"list": {
			hcltest.MockExprLiteral(cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("a")})),
			[]WantedError{
				{
					"Invalid enabled argument",
					"The given \"enabled\" argument value is unsuitable: bool required, but have list of string.",
				},
			},
		},
		"tuple": {
			hcltest.MockExprLiteral(cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")})),
			[]WantedError{
				{
					"Invalid enabled argument",
					"The given \"enabled\" argument value is unsuitable: bool required, but have tuple.",
				},
			},
		},
		"unknown": {
			hcltest.MockExprLiteral(cty.UnknownVal(cty.Number)),
			[]WantedError{
				{
					"Invalid enabled argument",
					`The given "enabled" argument value is derived from a value that won't be known until the apply phase, so OpenTofu cannot determine whether an instance of this object is declared or not.`,
				},
			},
		},
		"sensitive": {
			hcltest.MockExprLiteral(cty.StringVal("1").Mark(marks.Sensitive)),
			[]WantedError{
				{
					"Invalid enabled argument",
					`Sensitive values, or values derived from sensitive values, cannot be used in "enabled" arguments. If used, the sensitive value would be exposed by whether an instance is present.`,
				},
			},
		},
		"ephemeral": {
			hcltest.MockExprLiteral(cty.StringVal("1").Mark(marks.Ephemeral)),
			[]WantedError{
				{
					"Invalid enabled argument",
					`The given "enabled" argument value is unsuitable: the given value is ephemeral.`,
				},
			},
		},
		"bool conversion": {
			&hclsyntax.BinaryOpExpr{
				LHS: &hclsyntax.LiteralValueExpr{
					Val: cty.StringVal("3"),
				},
				RHS: &hclsyntax.LiteralValueExpr{
					Val: cty.StringVal("5"),
				},
				Op: hclsyntax.OpSubtract,
			},
			[]WantedError{
				{
					"Invalid enabled argument",
					`The given "enabled" argument value is unsuitable: bool required, but have number.`,
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, diags := EvaluateEnabledExpression(test.expr, mockRefsFunc())

			if len(diags) != len(test.Wanted) {
				t.Fatalf("wrong diagnostics size: (want %d, got %d):\n", len(test.Wanted), len(diags))
			}

			for i, wanted := range test.Wanted {
				diag := diags[i]
				if diff := cmp.Diff(diag.Description().Summary, wanted.Summary); diff != "" {
					t.Errorf("%d: wrong summary (-want +got):\n%s", i, diff)
				}

				if diff := cmp.Diff(diag.Description().Detail, wanted.DetailSubstring); diff != "" {
					t.Errorf("%d: wrong description (-want +got):\n%s", i, diff)
				}
			}

		})
	}
}
