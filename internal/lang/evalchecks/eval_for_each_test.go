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
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// returns a mock ref function for the unit tests
func mockRefsFunc() ContextFunc {
	return func(_ []*addrs.Reference) (*hcl.EvalContext, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		evalContext := hcl.EvalContext{}
		return &evalContext, diags
	}
}

func TestEvaluateForEachExpression(t *testing.T) {
	tests := map[string]struct {
		Expr       hcl.Expression
		ForEachMap map[string]cty.Value
	}{
		"empty set": {
			hcltest.MockExprLiteral(cty.SetValEmpty(cty.String)),
			map[string]cty.Value{},
		},
		"multi-value string set": {
			hcltest.MockExprLiteral(cty.SetVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")})),
			map[string]cty.Value{
				"a": cty.StringVal("a"),
				"b": cty.StringVal("b"),
			},
		},
		"empty map": {
			hcltest.MockExprLiteral(cty.MapValEmpty(cty.Bool)),
			map[string]cty.Value{},
		},
		"map": {
			hcltest.MockExprLiteral(cty.MapVal(map[string]cty.Value{
				"a": cty.BoolVal(true),
				"b": cty.BoolVal(false),
			})),
			map[string]cty.Value{
				"a": cty.BoolVal(true),
				"b": cty.BoolVal(false),
			},
		},
		"map containing unknown values": {
			hcltest.MockExprLiteral(cty.MapVal(map[string]cty.Value{
				"a": cty.UnknownVal(cty.Bool),
				"b": cty.UnknownVal(cty.Bool),
			})),
			map[string]cty.Value{
				"a": cty.UnknownVal(cty.Bool),
				"b": cty.UnknownVal(cty.Bool),
			},
		},
		"map containing sensitive values, but strings are literal": {
			hcltest.MockExprLiteral(cty.MapVal(map[string]cty.Value{
				"a": cty.BoolVal(true).Mark(marks.Sensitive),
				"b": cty.BoolVal(false),
			})),
			map[string]cty.Value{
				"a": cty.BoolVal(true).Mark(marks.Sensitive),
				"b": cty.BoolVal(false),
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			forEachMap, diags := EvaluateForEachExpression(test.Expr, mockRefsFunc())

			if len(diags) != 0 {
				t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
			}

			if !reflect.DeepEqual(forEachMap, test.ForEachMap) {
				t.Errorf(
					"wrong map value\ngot:  %swant: %s",
					spew.Sdump(forEachMap), spew.Sdump(test.ForEachMap),
				)
			}
		})
	}
}

func TestEvaluateForEachExpression_errors(t *testing.T) {
	tests := map[string]struct {
		Expr                               hcl.Expression
		Summary, DetailSubstring           string
		CausedByUnknown, CausedBySensitive bool
	}{
		"null set": {
			hcltest.MockExprLiteral(cty.NullVal(cty.Set(cty.String))),
			"Invalid for_each argument",
			`the given "for_each" argument value is null`,
			false, false,
		},
		"string": {
			hcltest.MockExprLiteral(cty.StringVal("i am definitely a set")),
			"Invalid for_each argument",
			"must be a map, or set of strings, and you have provided a value of type string",
			false, false,
		},
		"list": {
			hcltest.MockExprLiteral(cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("a")})),
			"Invalid for_each argument",
			"must be a map, or set of strings, and you have provided a value of type list",
			false, false,
		},
		"tuple": {
			hcltest.MockExprLiteral(cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")})),
			"Invalid for_each argument",
			"must be a map, or set of strings, and you have provided a value of type tuple",
			false, false,
		},
		"unknown string set": {
			hcltest.MockExprLiteral(cty.UnknownVal(cty.Set(cty.String))),
			"Invalid for_each argument",
			"set includes values derived from resource attributes that cannot be determined until apply",
			true, false,
		},
		"unknown map": {
			hcltest.MockExprLiteral(cty.UnknownVal(cty.Map(cty.Bool))),
			"Invalid for_each argument",
			"map includes keys derived from resource attributes that cannot be determined until apply",
			true, false,
		},
		"unknown pseudo-type": {
			hcltest.MockExprLiteral(cty.UnknownVal(cty.DynamicPseudoType)),
			"Invalid for_each argument",
			"map includes keys derived from resource attributes that cannot be determined until apply",
			true, false,
		},
		"marked map": {
			hcltest.MockExprLiteral(cty.MapVal(map[string]cty.Value{
				"a": cty.BoolVal(true),
				"b": cty.BoolVal(false),
			}).Mark(marks.Sensitive)),
			"Invalid for_each argument",
			"Sensitive values, or values derived from sensitive values, cannot be used as for_each arguments. If used, the sensitive value could be exposed as a resource instance key.",
			false, true,
		},
		"set containing booleans": {
			hcltest.MockExprLiteral(cty.SetVal([]cty.Value{cty.BoolVal(true)})),
			"Invalid for_each set argument",
			"supports sets of strings, but you have provided a set containing type bool",
			false, false,
		},
		"set containing null": {
			hcltest.MockExprLiteral(cty.SetVal([]cty.Value{cty.NullVal(cty.String)})),
			"Invalid for_each set argument",
			"must not contain null values",
			false, false,
		},
		"set containing unknown value": {
			hcltest.MockExprLiteral(cty.SetVal([]cty.Value{cty.UnknownVal(cty.String)})),
			"Invalid for_each argument",
			"set includes values derived from resource attributes that cannot be determined until apply",
			true, false,
		},
		"set containing dynamic unknown value": {
			hcltest.MockExprLiteral(cty.SetVal([]cty.Value{cty.UnknownVal(cty.DynamicPseudoType)})),
			"Invalid for_each argument",
			"set includes values derived from resource attributes that cannot be determined until apply",
			true, false,
		},
		"set containing marked values": {
			hcltest.MockExprLiteral(cty.SetVal([]cty.Value{cty.StringVal("beep").Mark(marks.Sensitive), cty.StringVal("boop")})),
			"Invalid for_each argument",
			"Sensitive values, or values derived from sensitive values, cannot be used as for_each arguments. If used, the sensitive value could be exposed as a resource instance key.",
			false, true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, diags := EvaluateForEachExpression(test.Expr, mockRefsFunc())

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
			if fromExpr := diags[0].FromExpr(); fromExpr != nil {
				if fromExpr.Expression == nil {
					t.Errorf("diagnostic does not refer to an expression")
				}
				if fromExpr.EvalContext == nil {
					t.Errorf("diagnostic does not refer to an EvalContext")
				}
			} else {
				t.Errorf("diagnostic does not support FromExpr\ngot: %s", spew.Sdump(diags[0]))
			}

			if got, want := tfdiags.DiagnosticCausedByUnknown(diags[0]), test.CausedByUnknown; got != want {
				t.Errorf("wrong result from tfdiags.DiagnosticCausedByUnknown\ngot:  %#v\nwant: %#v", got, want)
			}
			if got, want := tfdiags.DiagnosticCausedBySensitive(diags[0]), test.CausedBySensitive; got != want {
				t.Errorf("wrong result from tfdiags.DiagnosticCausedBySensitive\ngot:  %#v\nwant: %#v", got, want)
			}
		})
	}
}

func TestEvaluateForEachExpression_multi_errors(t *testing.T) {
	tests := map[string]struct {
		Expr   hcl.Expression
		Wanted []struct {
			Summary           string
			DetailSubstring   string
			CausedBySensitive bool
		}
	}{
		"marked list": {
			hcltest.MockExprLiteral(cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("a")}).Mark(marks.Sensitive)),
			[]struct {
				Summary           string
				DetailSubstring   string
				CausedBySensitive bool
			}{
				{
					"Invalid for_each argument",
					"Sensitive values, or values derived from sensitive values, cannot be used as for_each arguments. If used, the sensitive value could be exposed as a resource instance key.",
					true,
				},
				{
					"Invalid for_each argument",
					"must be a map, or set of strings, and you have provided a value of type list",
					false,
				},
			},
		},
		"marked tuple": {
			hcltest.MockExprLiteral(cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.StringVal("a")}).Mark(marks.Sensitive)),
			[]struct {
				Summary           string
				DetailSubstring   string
				CausedBySensitive bool
			}{
				{
					"Invalid for_each argument",
					"Sensitive values, or values derived from sensitive values, cannot be used as for_each arguments. If used, the sensitive value could be exposed as a resource instance key.",
					true,
				},
				{
					"Invalid for_each argument",
					"must be a map, or set of strings, and you have provided a value of type tuple",
					false,
				},
			},
		},
		"marked string": {
			hcltest.MockExprLiteral(cty.StringVal("a").Mark(marks.Sensitive)),
			[]struct {
				Summary           string
				DetailSubstring   string
				CausedBySensitive bool
			}{
				{
					"Invalid for_each argument",
					"Sensitive values, or values derived from sensitive values, cannot be used as for_each arguments. If used, the sensitive value could be exposed as a resource instance key.",
					true,
				},
				{
					"Invalid for_each argument",
					"must be a map, or set of strings, and you have provided a value of type string",
					false,
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, diags := EvaluateForEachExpression(test.Expr, mockRefsFunc())
			if len(diags) != len(test.Wanted) {
				t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
			}
			for idx := range test.Wanted {
				if got, want := diags[idx].Severity(), tfdiags.Error; got != want {
					t.Errorf("wrong diagnostic severity %#v; want %#v", got, want)
				}
				if got, want := diags[idx].Description().Summary, test.Wanted[idx].Summary; got != want {
					t.Errorf("wrong diagnostic summary\ngot:  %s\nwant: %s", got, want)
				}
				if got, want := diags[idx].Description().Detail, test.Wanted[idx].DetailSubstring; !strings.Contains(got, want) {
					t.Errorf("wrong diagnostic detail\ngot: %s\nwant substring: %s", got, want)
				}
				if fromExpr := diags[idx].FromExpr(); fromExpr != nil {
					if fromExpr.Expression == nil {
						t.Errorf("diagnostic does not refer to an expression")
					}
					if fromExpr.EvalContext == nil {
						t.Errorf("diagnostic does not refer to an EvalContext")
					}
				} else {
					t.Errorf("diagnostic does not support FromExpr\ngot: %s", spew.Sdump(diags[idx]))
				}

				if got, want := tfdiags.DiagnosticCausedBySensitive(diags[idx]), test.Wanted[idx].CausedBySensitive; got != want {
					t.Errorf("wrong result from tfdiags.DiagnosticCausedBySensitive\ngot:  %#v\nwant: %#v", got, want)
				}
			}
		})
	}
}

func TestEvaluateForEachExpressionKnown(t *testing.T) {
	tests := map[string]hcl.Expression{
		"unknown string set":  hcltest.MockExprLiteral(cty.UnknownVal(cty.Set(cty.String))),
		"unknown map":         hcltest.MockExprLiteral(cty.UnknownVal(cty.Map(cty.Bool))),
		"unknown tuple":       hcltest.MockExprLiteral(cty.UnknownVal(cty.Tuple([]cty.Type{cty.String, cty.Number, cty.Bool}))),
		"unknown pseudo-type": hcltest.MockExprLiteral(cty.UnknownVal(cty.DynamicPseudoType)),
	}

	for name, expr := range tests {
		t.Run(name, func(t *testing.T) {
			forEachVal, diags := EvaluateForEachExpressionValue(expr, mockRefsFunc(), true, true)

			if len(diags) != 0 {
				t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
			}

			if forEachVal.IsKnown() {
				t.Error("got known, want unknown")
			}
		})
	}
}

func TestEvaluateForEachExpressionValueTuple(t *testing.T) {
	tests := map[string]struct {
		Expr          hcl.Expression
		AllowTuple    bool
		ExpectedError string
	}{
		"valid tuple": {
			Expr:       hcltest.MockExprLiteral(cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")})),
			AllowTuple: true,
		},
		"empty tuple": {
			Expr:       hcltest.MockExprLiteral(cty.EmptyTupleVal),
			AllowTuple: true,
		},
		"null tuple": {
			Expr:          hcltest.MockExprLiteral(cty.NullVal(cty.Tuple([]cty.Type{}))),
			AllowTuple:    true,
			ExpectedError: "the given \"for_each\" argument value is null",
		},
		"sensitive tuple": {
			Expr:          hcltest.MockExprLiteral(cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}).Mark(marks.Sensitive)),
			AllowTuple:    true,
			ExpectedError: "Sensitive values, or values derived from sensitive values, cannot be used as for_each arguments",
		},
		"allow tuple is off": {
			Expr:          hcltest.MockExprLiteral(cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")})),
			AllowTuple:    false,
			ExpectedError: "the \"for_each\" argument must be a map, or set of strings, and you have provided a value of type tuple.",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, diags := EvaluateForEachExpressionValue(test.Expr, mockRefsFunc(), true, test.AllowTuple)

			if test.ExpectedError == "" {
				if len(diags) != 0 {
					t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
				}
			} else {
				if got, want := diags[0].Description().Detail, test.ExpectedError; test.ExpectedError != "" && !strings.Contains(got, want) {
					t.Errorf("wrong diagnostic detail\ngot: %s\nwant substring: %s", got, want)
				}
			}
		})
	}
}
