// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package evalchecks

import (
	"fmt"
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
			_, diags := EvaluateForEachExpression(test.Expr, mockRefsFunc(), nil)
			if len(diags) != len(test.Wanted) {
				t.Fatalf("Expected diagnostics: %s\nGot: %s\n", spew.Sdump(test.Wanted), spew.Sdump(diags))
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

func TestEvaluateForEachExpressionValueTuple(t *testing.T) {
	// Tests for cases where allowTuple is true. This can't be added to the tests on TestEvaluateForEach since they are called with allowTuple as false.
	tests := map[string]struct {
		Value        cty.Value
		expectedErrs []expectedErr
	}{
		"valid tuple": {
			Value: cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
		},
		"empty tuple": {
			Value: cty.EmptyTupleVal,
		},
		"null tuple": {
			Value: cty.NullVal(cty.Tuple([]cty.Type{})),
			expectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "the given \"for_each\" argument value is null",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
		},
		"sensitive tuple": {
			Value: cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}).Mark(marks.Sensitive),
			expectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "Sensitive values, or values derived from sensitive values, cannot be used as for_each arguments",
					CausedByUnknown:   false,
					CausedBySensitive: true,
				},
			},
		},
		"unknown tuple": {
			Value: cty.UnknownVal(cty.Tuple([]cty.Type{cty.String})),
			expectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "tuple includes values derived from resource attributes that cannot be determined until apply",
					CausedByUnknown:   true,
					CausedBySensitive: false,
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			expr := hcltest.MockExprLiteral(test.Value)
			_, diags := EvaluateForEachExpressionValue(expr, mockRefsFunc(), false, true, nil)

			assertDiagnosticsMatch(t, diags, test.expectedErrs, "plan")
		})
	}
}

func TestForEachCommandLineExcludeSuggestion(t *testing.T) {
	noKeyResourceAddr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "happycloud_virtual_machine",
		Name: "boop",
	}.Absolute(addrs.RootModuleInstance)
	stringKeyResourceAddr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "happycloud_virtual_machine",
		Name: "boop",
	}.Absolute(addrs.RootModuleInstance.Child("foo", addrs.StringKey("bleep bloop")))

	tests := []struct {
		excludableAddr addrs.Targetable
		goos           string
		want           string
	}{
		{
			nil,
			"linux",
			`Alternatively, you could use the -target option to first apply only the resources that for_each depends on, and then apply normally to converge.`,
		},
		{
			nil,
			"windows",
			`Alternatively, you could use the -target option to first apply only the resources that for_each depends on, and then apply normally to converge.`,
		},
		{
			nil,
			"darwin",
			`Alternatively, you could use the -target option to first apply only the resources that for_each depends on, and then apply normally to converge.`,
		},
		{
			noKeyResourceAddr,
			"linux",
			`Alternatively, you could use the planning option -exclude=happycloud_virtual_machine.boop to first apply without this object, and then apply normally to converge.`,
		},
		{
			noKeyResourceAddr,
			"windows",
			`Alternatively, you could use the planning option -exclude=happycloud_virtual_machine.boop to first apply without this object, and then apply normally to converge.`,
		},
		{
			noKeyResourceAddr,
			"darwin",
			`Alternatively, you could use the planning option -exclude=happycloud_virtual_machine.boop to first apply without this object, and then apply normally to converge.`,
		},
		{
			stringKeyResourceAddr,
			"linux",
			`Alternatively, you could use the planning option -exclude='module.foo["bleep bloop"].happycloud_virtual_machine.boop' to first apply without this object, and then apply normally to converge.`,
		},
		{
			stringKeyResourceAddr,
			"windows",
			`Alternatively, you could use the planning option -exclude="module.foo[\"bleep bloop\"].happycloud_virtual_machine.boop" to first apply without this object, and then apply normally to converge.`,
		},
		{
			stringKeyResourceAddr,
			"darwin",
			`Alternatively, you could use the planning option -exclude='module.foo["bleep bloop"].happycloud_virtual_machine.boop' to first apply without this object, and then apply normally to converge.`,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%v on %s", test.excludableAddr, test.goos), func(t *testing.T) {
			got := forEachCommandLineExcludeSuggestionImpl(test.excludableAddr, test.goos)
			if got != test.want {
				t.Errorf("wrong suggestion\ngot:  %s\nwant: %s", got, test.want)
			}
		})
	}
}

type expectedErr struct {
	Summary           string
	Detail            string
	CausedByUnknown   bool
	CausedBySensitive bool
}

func assertDiagnosticsMatch(t *testing.T, gotDiags []tfdiags.Diagnostic, wantDiags []expectedErr, phase string) {
	t.Helper()
	if len(gotDiags) != len(wantDiags) {
		t.Errorf("got %d errors in %s phase; expected %d", len(gotDiags), phase, len(wantDiags))
	}

	for i := range max(len(gotDiags), len(wantDiags)) {
		if i >= len(gotDiags) {
			t.Errorf("on %s phase, got no error on position %d, while expecting error:\n%s\n%s", phase, i, wantDiags[i].Summary, wantDiags[i].Detail)
		} else if i >= len(wantDiags) {
			t.Errorf("on %s phase on position %d, while expecting no error, got:\n%s\n%s", phase, i, gotDiags[i].Description().Summary, gotDiags[i].Description().Detail)
		} else {
			assertDiagnosticMatch(t, phase, gotDiags[i], wantDiags[i])
		}
	}
}

func assertDiagnosticMatch(t *testing.T, phase string, gotDiag tfdiags.Diagnostic, wantDiag expectedErr) {
	t.Helper()

	if got, want := gotDiag.Severity(), tfdiags.Error; got != want {
		t.Errorf("wrong diagnostic severity on %s phase %#v; want %#v", phase, got, want)
	}

	if got, want := gotDiag.Description().Summary, wantDiag.Summary; got != want {
		t.Errorf("wrong diagnostic summary on %s phase\ngot:  %s\nwant: %s", phase, got, want)
	}

	if got, want := gotDiag.Description().Detail, wantDiag.Detail; !strings.Contains(got, want) {
		t.Errorf("wrong diagnostic detail on %s phase, \ngot: %s\nwant substring: %s", phase, got, want)
	}

	if got, want := tfdiags.DiagnosticCausedByUnknown(gotDiag), wantDiag.CausedByUnknown; got != want {
		t.Errorf("wrong result from tfdiags.DiagnosticCausedByUnknown on %s phase\ngot:  %#v\nwant: %#v", phase, got, want)
	}
	if got, want := tfdiags.DiagnosticCausedBySensitive(gotDiag), wantDiag.CausedBySensitive; got != want {
		t.Errorf("wrong result from tfdiags.DiagnosticCausedBySensitive on %s phase\ngot:  %#v\nwant: %#v", phase, got, want)
	}

	if fromExpr := gotDiag.FromExpr(); fromExpr != nil {
		if fromExpr.Expression == nil {
			t.Errorf("diagnostic does not refer to an expression")
		}
		if fromExpr.EvalContext == nil {
			t.Errorf("diagnostic does not refer to an EvalContext")
		}
	} else {
		t.Errorf("diagnostic does not support FromExpr\ngot: %s", spew.Sdump(gotDiag))
	}
}

func TestEvaluateForEach(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Input                cty.Value
		ValidateExpectedErrs []expectedErr
		ValidateReturnValue  cty.Value
		PlanExpectedErrs     []expectedErr
		PlanReturnValue      map[string]cty.Value
	}{
		"empty_set": {
			Input:                cty.SetValEmpty(cty.String),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.SetValEmpty(cty.String),
			PlanExpectedErrs:     nil,
			PlanReturnValue:      map[string]cty.Value{},
		},
		"empty_set_dynamic": {
			Input:                cty.SetValEmpty(cty.DynamicPseudoType),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.SetValEmpty(cty.DynamicPseudoType),
			PlanExpectedErrs:     nil,
			PlanReturnValue:      map[string]cty.Value{},
		},
		"set_of_strings": {
			Input:                cty.SetVal([]cty.Value{cty.StringVal("a")}),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.SetVal([]cty.Value{cty.StringVal("a")}),
			PlanExpectedErrs:     nil,
			PlanReturnValue:      map[string]cty.Value{"a": cty.StringVal("a")},
		},
		"set_of_bool": {
			Input: cty.SetVal([]cty.Value{cty.True}),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "but you have provided a set containing type bool.",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.Set(cty.Bool)),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "but you have provided a set containing type bool.",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"set_of_null": {
			Input: cty.SetVal([]cty.Value{cty.NullVal(cty.String)}),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "sets must not contain null values",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.Set(cty.String)),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "sets must not contain null values.",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"set_of_unknown_strings": {
			Input:                cty.SetVal([]cty.Value{cty.UnknownVal(cty.String)}),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.UnknownVal(cty.Set(cty.String)),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "set includes values derived from resource attributes that cannot be determined until apply",
					CausedByUnknown:   true,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"set_of_unknown_dynamic": {
			Input:                cty.SetVal([]cty.Value{cty.DynamicVal}),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.UnknownVal(cty.Set(cty.DynamicPseudoType)),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "set includes values derived from resource attributes that cannot be determined until apply",
					CausedByUnknown:   true,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		// # Tuples
		"empty_tuple": {
			Input: cty.EmptyTupleVal,
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.EmptyTuple),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"tuple_of_strings": {
			Input: cty.TupleVal([]cty.Value{cty.StringVal("a")}),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.Tuple([]cty.Type{cty.String})),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"tuple_of_bool": {
			Input: cty.TupleVal([]cty.Value{cty.True}),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.Tuple([]cty.Type{cty.Bool})),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"tuple_of_null": {
			Input: cty.TupleVal([]cty.Value{cty.NullVal(cty.String)}),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.Tuple([]cty.Type{cty.String})),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"tuple_of_unknown_strings": {
			Input: cty.TupleVal([]cty.Value{cty.UnknownVal(cty.String)}),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.Tuple([]cty.Type{cty.String})),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"tuple_of_unknown_dynamic": {
			Input: cty.TupleVal([]cty.Value{cty.DynamicVal}),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.Tuple([]cty.Type{cty.DynamicPseudoType})),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"empty_map": {
			Input:                cty.MapValEmpty(cty.String),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.MapValEmpty(cty.String),
			PlanExpectedErrs:     nil,
			PlanReturnValue:      map[string]cty.Value{},
		},
		"map_of_null": {
			Input:                cty.MapVal(map[string]cty.Value{"a": cty.NullVal(cty.String)}),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.MapVal(map[string]cty.Value{"a": cty.NullVal(cty.String)}),
			PlanExpectedErrs:     nil,
			PlanReturnValue:      map[string]cty.Value{"a": cty.NullVal(cty.String)},
		},
		"map_of_unknown_string": {
			Input:                cty.MapVal(map[string]cty.Value{"a": cty.UnknownVal(cty.String)}),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.MapVal(map[string]cty.Value{"a": cty.UnknownVal(cty.String)}),
			PlanExpectedErrs:     nil,
			PlanReturnValue:      map[string]cty.Value{"a": cty.UnknownVal(cty.String)},
		},
		"map_of_bool": {
			Input:                cty.MapVal(map[string]cty.Value{"a": cty.True}),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.MapVal(map[string]cty.Value{"a": cty.True}),
			PlanExpectedErrs:     nil,
			PlanReturnValue:      map[string]cty.Value{"a": cty.True},
		},
		"map_of_unknown_bool": {
			Input:                cty.MapVal(map[string]cty.Value{"a": cty.UnknownVal(cty.Bool)}),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.MapVal(map[string]cty.Value{"a": cty.UnknownVal(cty.Bool)}),
			PlanExpectedErrs:     nil,
			PlanReturnValue:      map[string]cty.Value{"a": cty.UnknownVal(cty.Bool)},
		},
		"map_of_string": {
			Input:                cty.MapVal(map[string]cty.Value{"a": cty.StringVal("b")}),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.MapVal(map[string]cty.Value{"a": cty.StringVal("b")}),
			PlanExpectedErrs:     nil,
			PlanReturnValue:      map[string]cty.Value{"a": cty.StringVal("b")},
		},
		"empty_object": {
			Input:                cty.EmptyObjectVal,
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.EmptyObjectVal,
			PlanExpectedErrs:     nil,
			PlanReturnValue:      map[string]cty.Value{},
		},
		"object_with_null_values": {
			Input:                cty.ObjectVal(map[string]cty.Value{"a": cty.NullVal(cty.String)}),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.ObjectVal(map[string]cty.Value{"a": cty.NullVal(cty.String)}),
			PlanExpectedErrs:     nil,
			PlanReturnValue:      map[string]cty.Value{"a": cty.NullVal(cty.String)},
		},
		"object_with_unknown_string_values": {
			Input:                cty.ObjectVal(map[string]cty.Value{"a": cty.UnknownVal(cty.String)}),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.ObjectVal(map[string]cty.Value{"a": cty.UnknownVal(cty.String)}),
			PlanExpectedErrs:     nil,
			PlanReturnValue:      map[string]cty.Value{"a": cty.UnknownVal(cty.String)},
		},
		"object_with_bool_values": {
			Input:                cty.ObjectVal(map[string]cty.Value{"a": cty.BoolVal(true)}),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.ObjectVal(map[string]cty.Value{"a": cty.True}),
			PlanExpectedErrs:     nil,
			PlanReturnValue:      map[string]cty.Value{"a": cty.True},
		},
		"object_with_unknown_bool_values": {
			Input:                cty.ObjectVal(map[string]cty.Value{"a": cty.UnknownVal(cty.Bool)}),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.ObjectVal(map[string]cty.Value{"a": cty.UnknownVal(cty.Bool)}),
			PlanExpectedErrs:     nil,
			PlanReturnValue:      map[string]cty.Value{"a": cty.UnknownVal(cty.Bool)},
		},
		"object_with_string_values": {
			Input:                cty.ObjectVal(map[string]cty.Value{"a": cty.StringVal("b")}),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.ObjectVal(map[string]cty.Value{"a": cty.StringVal("b")}),
			PlanExpectedErrs:     nil,
			PlanReturnValue:      map[string]cty.Value{"a": cty.StringVal("b")},
		},
		// Other
		"null_set_string": {
			Input: cty.NullVal(cty.Set(cty.String)),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "argument value is null.",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.Set(cty.String)),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "argument value is null.",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"null_object": {
			Input: cty.NullVal(cty.Object(map[string]cty.Type{
				"route_addrs": cty.String,
				"cidr":        cty.String,
			})),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "argument value is null.",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.Object(map[string]cty.Type{"cidr": cty.String, "route_addrs": cty.String})),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "argument value is null.",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"null_tuple": {
			Input: cty.NullVal(cty.Tuple([]cty.Type{})),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "argument value is null.",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
				{
					Summary:           "Invalid for_each argument",
					Detail:            "argument must be a map, or set of strings, and you have provided a value of type tuple.",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.Tuple([]cty.Type{})),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "argument value is null.",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
				{
					Summary:           "Invalid for_each argument",
					Detail:            "argument must be a map, or set of strings, and you have provided a value of type tuple.",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"sensitive_tuple": {
			Input: cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}).Mark(marks.Sensitive),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "Sensitive values, or values derived from sensitive values, cannot be used as for_each arguments",
					CausedByUnknown:   false,
					CausedBySensitive: true,
				},
				{
					Summary:           "Invalid for_each argument",
					Detail:            "argument must be a map, or set of strings, and you have provided a value of type tuple.",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.Tuple([]cty.Type{cty.String, cty.String})),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "Sensitive values, or values derived from sensitive values, cannot be used as for_each arguments",
					CausedByUnknown:   false,
					CausedBySensitive: true,
				},
				{
					Summary:           "Invalid for_each argument",
					Detail:            "argument must be a map, or set of strings, and you have provided a value of type tuple.",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"string": {
			Input: cty.StringVal("i am definitely a set"),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type string",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.String),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type string",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"number": {
			Input: cty.MustParseNumberVal("1e+50"),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type number",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.Number),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type number",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"bool": {
			Input: cty.True,
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type bool",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.Bool),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type bool",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"list": {
			Input: cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("a")}),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type list of string",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.List(cty.String)),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "must be a map, or set of strings, and you have provided a value of type list of string",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		// Top-Level Unknowns (basically all the above with unknown on the top-level)
		"unknown_empty_set": {
			Input:                cty.UnknownVal(cty.Set(cty.String)),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.UnknownVal(cty.Set(cty.String)),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "set includes values derived from resource attributes that cannot be determined until apply",
					CausedByUnknown:   true,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"unknown_set_of_object": {
			Input: cty.UnknownVal(cty.Set(cty.Object(map[string]cty.Type{
				"route_addrs": cty.String,
				"cidr":        cty.String,
			}))),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "set containing type object.",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.UnknownVal(cty.Set(cty.Object(map[string]cty.Type{"cidr": cty.String, "route_addrs": cty.String}))),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "set includes values derived from resource attributes that cannot be determined until apply",
					CausedByUnknown:   true,
					CausedBySensitive: false,
				},
				{
					Summary:           "Invalid for_each argument",
					Detail:            "set containing type object.",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"unknown_set_of_strings": {
			Input:                cty.UnknownVal(cty.Set(cty.String)),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.UnknownVal(cty.Set(cty.String)),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "set includes values derived from resource attributes that cannot be determined until apply",
					CausedByUnknown:   true,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"unknown_set_of_bool": {
			Input: cty.UnknownVal(cty.Set(cty.Bool)),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "set containing type bool.",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.UnknownVal(cty.Set(cty.Bool)),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "set includes values derived from resource attributes that cannot be determined until apply",
					CausedByUnknown:   true,
					CausedBySensitive: false,
				},
				{
					Summary:           "Invalid for_each argument",
					Detail:            "set containing type bool.",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"unknown_set_of_dynamic": {
			Input:                cty.UnknownVal(cty.Set(cty.DynamicPseudoType)),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.UnknownVal(cty.Set(cty.DynamicPseudoType)),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "set includes values derived from resource attributes that cannot be determined until apply",
					CausedByUnknown:   true,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"unknown_empty_map": {
			Input:                cty.UnknownVal(cty.Map(cty.String)),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.UnknownVal(cty.Map(cty.String)),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "map includes keys derived from resource attributes that cannot be determined until apply",
					CausedByUnknown:   true,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"unknown_map_of_strings": {
			Input:                cty.UnknownVal(cty.Map(cty.String)),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.UnknownVal(cty.Map(cty.String)),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "map includes keys derived from resource attributes that cannot be determined until apply",
					CausedByUnknown:   true,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"unknown_of_unknowns": {
			Input:                cty.DynamicVal,
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.DynamicVal,
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "value includes keys or set values from resource attributes that cannot be determined until apply",
					CausedByUnknown:   true,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"unknown_map_of_bool": {
			Input:                cty.UnknownVal(cty.Map(cty.Bool)),
			ValidateExpectedErrs: nil,
			ValidateReturnValue:  cty.UnknownVal(cty.Map(cty.Bool)),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "map includes keys derived from resource attributes that cannot be determined until apply",
					CausedByUnknown:   true,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"unknown_tuple_of_strings": {
			Input: cty.UnknownVal(cty.Tuple([]cty.Type{cty.String})),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "argument must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.Tuple([]cty.Type{cty.String})),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "value includes keys or set values from resource attributes that cannot be determined until apply",
					CausedByUnknown:   true,
					CausedBySensitive: false,
				},
				{
					Summary:           "Invalid for_each argument",
					Detail:            "argument must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"unknown_tuple_of_dynamic": {
			Input: cty.UnknownVal(cty.Tuple([]cty.Type{cty.DynamicPseudoType})),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "argument must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.Tuple([]cty.Type{cty.DynamicPseudoType})),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "value includes keys or set values from resource attributes that cannot be determined until apply",
					CausedByUnknown:   true,
					CausedBySensitive: false,
				},
				{
					Summary:           "Invalid for_each argument",
					Detail:            "argument must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
		"unknown_tuple_of_bool": {
			Input: cty.UnknownVal(cty.Tuple([]cty.Type{cty.Bool})),
			ValidateExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "argument must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			ValidateReturnValue: cty.NullVal(cty.Tuple([]cty.Type{cty.Bool})),
			PlanExpectedErrs: []expectedErr{
				{
					Summary:           "Invalid for_each argument",
					Detail:            "value includes keys or set values from resource attributes that cannot be determined until apply",
					CausedByUnknown:   true,
					CausedBySensitive: false,
				},
				{
					Summary:           "Invalid for_each argument",
					Detail:            "argument must be a map, or set of strings, and you have provided a value of type tuple",
					CausedByUnknown:   false,
					CausedBySensitive: false,
				},
			},
			PlanReturnValue: map[string]cty.Value{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Validate Phase
			allowUnknown := true
			allowTuple := false
			expr := hcltest.MockExprLiteral(test.Input)
			validateReturn, validateDiags := EvaluateForEachExpressionValue(expr, mockRefsFunc(), allowUnknown, allowTuple, nil)

			if !validateReturn.RawEquals(test.ValidateReturnValue) {
				t.Errorf("got %#v in validate phase; want %#v", validateReturn, test.ValidateReturnValue)
			}

			if test.ValidateExpectedErrs != nil || len(validateDiags) > 0 {
				assertDiagnosticsMatch(t, validateDiags, test.ValidateExpectedErrs, "validate")
			}

			// Plan Phase
			planReturn, planDiags := EvaluateForEachExpression(expr, mockRefsFunc(), nil)

			if !reflect.DeepEqual(planReturn, test.PlanReturnValue) {
				t.Errorf("got %#v in plan phase; want %#v", planReturn, test.PlanReturnValue)
			}

			if test.PlanExpectedErrs != nil || len(planDiags) > 0 {
				assertDiagnosticsMatch(t, planDiags, test.PlanExpectedErrs, "plan")
			}
		})
	}
}
