// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package evalchecks

import (
	"reflect"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// returns a mock ref function for the unit tests
func mockEvaluateFunc(val cty.Value) EvaluateFunc {
	return func(_ hcl.Expression) (cty.Value, tfdiags.Diagnostics) {
		return val, tfdiags.Diagnostics{}
	}
}

func TestEvaluateCountExpression_valid(t *testing.T) {
	tests := map[string]struct {
		val      cty.Value
		expected int
	}{
		"1": {
			cty.NumberIntVal(1),
			1,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual, diags := EvaluateCountExpression(hcltest.MockExprLiteral(test.val), mockEvaluateFunc(test.val))

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

func TestEvaluateCountExpression_errors(t *testing.T) {
	tests := map[string]struct {
		val                      cty.Value
		Summary, DetailSubstring string
		CausedByUnknown          bool
	}{
		"null": {
			cty.NullVal(cty.Number),
			"Invalid count argument",
			`The given "count" argument value is null. An integer is required.`,
			false,
		},
		"negative": {
			cty.NumberIntVal(-1),
			"Invalid count argument",
			`The given "count" argument value is unsuitable: must be greater than or equal to zero.`,
			false,
		},
		"string": {
			cty.StringVal("i am definitely a number"),
			"Invalid count argument",
			"The given \"count\" argument value is unsuitable: number value is required.",
			false,
		},
		"list": {
			cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("a")}),
			"Invalid count argument",
			"The given \"count\" argument value is unsuitable: number value is required.",
			false,
		},
		"tuple": {
			cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
			"Invalid count argument",
			"The given \"count\" argument value is unsuitable: number value is required.",
			false,
		},
		"unknown": {
			cty.UnknownVal(cty.Number),
			"Invalid count argument",
			`he "count" value depends on resource attributes that cannot be determined until apply, so OpenTofu cannot predict how many instances will be created. To work around this, use the -target argument to first apply only the resources that the count depends on.`,
			true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, diags := EvaluateCountExpression(hcltest.MockExprLiteral(test.val), mockEvaluateFunc(test.val))

			if len(diags) != 1 {
				t.Fatalf("got %d diagnostics; want 1", diags)
			}
			if got, want := diags[0].Severity(), tfdiags.Error; got != want {
				t.Errorf("wrong diagnostic severity %#v; want %#v", got, want)
			}
			if got, want := diags[0].Description().Summary, test.Summary; got != want {
				t.Errorf("wrong diagnostic summary\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := diags[0].Description().Detail, test.DetailSubstring; !strings.Contains(got, want) {
				t.Errorf("wrong diagnostic detail\ngot: %s\nwant substring: %s", got, want)
			}

			if got, want := tfdiags.DiagnosticCausedByUnknown(diags[0]), test.CausedByUnknown; got != want {
				t.Errorf("wrong result from tfdiags.DiagnosticCausedByUnknown\ngot:  %#v\nwant: %#v", got, want)
			}
		})
	}
}
