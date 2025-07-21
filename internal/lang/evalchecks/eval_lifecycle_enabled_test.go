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
		val    cty.Value
		Wanted []WantedError
	}{
		"null": {
			cty.NullVal(cty.Number),
			[]WantedError{
				{
					"Invalid enabled argument",
					`The given "enabled" argument value is unsuitable. An integer is required.`,
				},
				{
					"Invalid enabled argument",
					`The given "enabled" argument value is unsuitable. An integer is required.`,
				},
			},
		},
		"1": {
			cty.NumberIntVal(1),
			[]WantedError{
				{
					"Invalid enabled argument",
					`The given "enabled" argument value is null. An integer is required.`,
				},
			},
		},
		"negative": {
			cty.NumberIntVal(-1),
			[]WantedError{
				{
					"Invalid enabled argument",
					`The given "enabled" argument value is unsuitable: must be greater than or equal to zero.`,
				},
			},
		},
		"list": {
			cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("a")}),
			[]WantedError{
				{
					"Invalid enabled argument",
					"The given \"enabled\" argument value is unsuitable: number value is required.",
				},
			},
		},
		"tuple": {
			cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
			[]WantedError{
				{
					"Invalid enabled argument",
					"The given \"enabled\" argument value is unsuitable: number value is required.",
				},
			},
		},
		"unknown": {
			cty.UnknownVal(cty.Number),
			[]WantedError{
				{
					"Invalid enabled argument",
					"The \"enabled\" value depends on resource attributes that cannot be determined until apply, so OpenTofu cannot predict how many instances will be created.\n\nTo work around this, use the -target option to first apply only the resources that the enabled depends on, and then apply normally to converge.",
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, diags := EvaluateEnabledExpression(hcltest.MockExprLiteral(test.val), mockRefsFunc())

			if len(diags) != len(test.Wanted) {
				t.Fatalf("wrong diagnostics size: (want %d, got %d):\n", len(test.Wanted), len(diags))
			}
			for i, diag := range diags {

				if diff := cmp.Diff(diag.Description().Summary, test.Wanted[i].Summary); diff != "" {
					t.Errorf("wrong diagnostics (-want +got):\n%s", diff)
				}

				if diff := cmp.Diff(diag.Description().Detail, test.Wanted[i].DetailSubstring); diff != "" {
					t.Errorf("wrong diagnostics (-want +got):\n%s", diff)
				}
			}

		})
	}
}
