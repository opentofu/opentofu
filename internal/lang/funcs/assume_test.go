// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package funcs

import (
	"fmt"
	"testing"

	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

func TestAssumeEqual(t *testing.T) {
	tests := []struct {
		// Note that when called through HCL the arguments get automatically
		// converted to the parameter type constraint, but we're testing
		// direct calls to the function here so we should only test values
		// that already have a suitable type.
		Actual, Assumed cty.Value
		WantValue       cty.Value
		WantError       string
	}{
		{
			Actual:    cty.UnknownVal(cty.String),
			Assumed:   cty.StringVal("hello"),
			WantValue: cty.StringVal("hello"),
		},
		{
			Actual:    cty.StringVal("hello"),
			Assumed:   cty.UnknownVal(cty.String),
			WantValue: cty.UnknownVal(cty.String),
		},
		{
			Actual:    cty.UnknownVal(cty.String),
			Assumed:   cty.UnknownVal(cty.String),
			WantValue: cty.UnknownVal(cty.String),
		},
		{
			Actual:    cty.DynamicVal, // automatically converted to match Assumed
			Assumed:   cty.UnknownVal(cty.String),
			WantValue: cty.UnknownVal(cty.String),
		},
		{
			Actual:    cty.StringVal("hello"),
			Assumed:   cty.StringVal("hello"),
			WantValue: cty.StringVal("hello"),
		},
		{
			Actual:    cty.StringVal("howdy"),
			Assumed:   cty.StringVal("hello"),
			WantError: `the actual value does not match the assumed value`,
		},
		{
			Actual:    cty.UnknownVal(cty.String).Mark("!"),
			Assumed:   cty.StringVal("hello"),
			WantValue: cty.StringVal("hello").Mark("!"),
		},
		{
			Actual:    cty.StringVal("hello").Mark("!"),
			Assumed:   cty.UnknownVal(cty.String),
			WantValue: cty.UnknownVal(cty.String).Mark("!"),
		},
		{
			Actual:    cty.StringVal("hello"),
			Assumed:   cty.UnknownVal(cty.String).Mark("!"),
			WantValue: cty.UnknownVal(cty.String).Mark("!"),
		},
		{
			Actual:    cty.StringVal("hello").Mark("!"),
			Assumed:   cty.StringVal("hello"),
			WantValue: cty.StringVal("hello").Mark("!"),
		},
		{
			Actual:    cty.StringVal("howdy").Mark("!"),
			Assumed:   cty.StringVal("hello"),
			WantError: `the actual value does not match the assumed value`,
		},
		{
			Actual:    cty.UnknownVal(cty.String),
			Assumed:   cty.StringVal("hello").Mark("!"),
			WantValue: cty.StringVal("hello").Mark("!"),
		},
		{
			Actual:    cty.StringVal("hello"),
			Assumed:   cty.StringVal("hello").Mark("!"),
			WantValue: cty.StringVal("hello").Mark("!"),
		},
		{
			Actual:    cty.UnknownVal(cty.String).Mark(marks.Sensitive),
			Assumed:   cty.StringVal("hello"),
			WantValue: cty.StringVal("hello"),
		},
		{
			Actual:    cty.StringVal("hello").Mark(marks.Sensitive),
			Assumed:   cty.StringVal("hello"),
			WantValue: cty.StringVal("hello"),
		},
		{
			Actual:    cty.UnknownVal(cty.String),
			Assumed:   cty.StringVal("hello").Mark(marks.Sensitive),
			WantValue: cty.StringVal("hello").Mark(marks.Sensitive),
		},
		{
			Actual:    cty.StringVal("hello"),
			Assumed:   cty.StringVal("hello").Mark(marks.Sensitive),
			WantValue: cty.StringVal("hello").Mark(marks.Sensitive),
		},
		{
			Actual:    cty.UnknownVal(cty.String).Mark(marks.Ephemeral),
			Assumed:   cty.StringVal("hello"),
			WantValue: cty.StringVal("hello"),
		},
		{
			Actual:    cty.UnknownVal(cty.String),
			Assumed:   cty.StringVal("hello").Mark(marks.Ephemeral),
			WantValue: cty.StringVal("hello").Mark(marks.Ephemeral),
		},
		{
			Actual: cty.ListVal([]cty.Value{ // automatically converted to cty.List(cty.String) to match Assumed
				cty.DynamicVal,
			}),
			Assumed: cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
			}),
			WantValue: cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
			}),
		},
		{
			Actual: cty.TupleVal([]cty.Value{ // automatically converted to cty.List(cty.String) to match Assumed
				cty.DynamicVal,
			}),
			Assumed: cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
			}),
			WantValue: cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
			}),
		},
		{
			Actual: cty.TupleVal([]cty.Value{ // automatically converted to cty.List(cty.String) to match Assumed
				cty.StringVal("hello"),
			}),
			Assumed: cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
			}),
			WantValue: cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
			}),
		},
		{
			Actual: cty.TupleVal([]cty.Value{
				cty.DynamicVal,
			}),
			Assumed: cty.ListVal([]cty.Value{
				cty.UnknownVal(cty.String),
			}),
			WantValue: cty.ListVal([]cty.Value{
				cty.UnknownVal(cty.String),
			}),
		},
		{
			Actual: cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
			}),
			Assumed: cty.TupleVal([]cty.Value{
				cty.StringVal("hello"),
			}),
			// list of string does not convert to single-element tuple in cty,
			// because historically such conversions caused some confusion
			// due to the unavoidable requirement that the source list must
			// have a length exactly matching the tuple length, and so for this
			// case where the actual value is a list the author would need to
			// use "tolist" or "convert" to explicitly convert the assumed
			// value to be a list, rather than having the actual value be
			// converted to a tuple.
			WantError: `actual value type list of string does not match assumed value type tuple`,
		},
		{
			Actual: cty.DynamicVal, // automatically converted to cty.List(cty.String) to match Assumed
			Assumed: cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
			}),
			WantValue: cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
			}),
		},
		{
			Actual: cty.UnknownVal(cty.String), // automatic conversion is impossible in this case
			Assumed: cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
			}),
			WantError: `actual value type string does not match assumed value type list of string`,
		},
		{
			Actual: cty.TupleVal([]cty.Value{
				cty.StringVal("hello").Mark("!"),
			}),
			Assumed: cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
			}),
			WantValue: cty.ListVal([]cty.Value{
				cty.StringVal("hello").Mark("!"),
			}),
		},
		{
			Actual: cty.TupleVal([]cty.Value{
				cty.StringVal("hello"),
			}),
			Assumed: cty.ListVal([]cty.Value{
				cty.StringVal("hello").Mark("!"),
			}),
			WantValue: cty.ListVal([]cty.Value{
				cty.StringVal("hello").Mark("!"),
			}),
		},
		{
			Actual: cty.TupleVal([]cty.Value{
				cty.StringVal("hello").Mark(marks.Sensitive),
			}),
			Assumed: cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
			}),
			WantValue: cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
			}),
		},
		{
			Actual: cty.TupleVal([]cty.Value{
				cty.StringVal("hello").Mark(marks.Ephemeral),
			}),
			Assumed: cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
			}),
			WantValue: cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
			}),
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%#v,%#v", test.Actual, test.Assumed), func(t *testing.T) {
			t.Logf("inputs:\nactual:  %#v\nassumed: %#v", test.Actual, test.Assumed)
			gotVal, gotErr := AssumeEqual(test.Actual, test.Assumed)

			if wantErr := test.WantError; wantErr != "" {
				if gotErr == nil {
					t.Fatalf("unexpected success\nwant error: %s\ngot result: %#v", wantErr, gotVal)
				}
				if gotErr := gotErr.Error(); gotErr != wantErr {
					t.Fatalf("unexpected error\ngot:  %s\nwant: %s", gotErr, wantErr)
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

func TestAssumeNotNull(t *testing.T) {
	tests := []struct {
		Input     cty.Value
		WantValue cty.Value
		WantError string
	}{
		{
			Input:     cty.UnknownVal(cty.String),
			WantValue: cty.UnknownVal(cty.String).RefineNotNull(),
		},
		{
			Input:     cty.StringVal("hello"),
			WantValue: cty.StringVal("hello"),
		},
		{
			Input:     cty.NullVal(cty.String),
			WantError: `assumption was not upheld`,
		},
		{
			Input:     cty.DynamicVal,
			WantError: `given value must have a known type; consider using the \"convert\" function to specify a type to assume`,
		},
		{
			Input:     cty.NullVal(cty.DynamicPseudoType),
			WantError: `given value must have a known type; consider using the \"convert\" function to specify a type to assume`,
		},
	}

	for _, test := range tests {
		t.Run(test.Input.GoString(), func(t *testing.T) {
			gotVal, gotErr := AssumeNotNull(test.Input)

			if wantErr := test.WantError; wantErr != "" {
				if gotErr == nil {
					t.Fatalf("unexpected success\nwant error: %s\ngot result: %#v", wantErr, gotVal)
				}
				if gotErr := gotErr.Error(); gotErr != wantErr {
					t.Fatalf("unexpected error\ngot:  %s\nwant: %s", gotErr, wantErr)
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

func TestAssumeStringPrefix(t *testing.T) {
	tests := []struct {
		// Note that when called through HCL the arguments get automatically
		// converted to the parameter type constraint, but we're testing
		// direct calls to the function here so we should only test values
		// that already have a suitable type.
		Input, Prefix cty.Value
		WantValue     cty.Value
		WantError     string
	}{
		{
			Input:  cty.UnknownVal(cty.String),
			Prefix: cty.StringVal("foo-"),
			WantValue: cty.UnknownVal(cty.String).Refine().
				StringPrefixFull("foo-").
				NewValue(),
		},
		{
			Input:  cty.UnknownVal(cty.String),
			Prefix: cty.StringVal("foo"),
			WantValue: cty.UnknownVal(cty.String).Refine().
				// the final "o" gets dropped because it could potentially
				// combine with subsequent diacritics, which would invalidate
				// the prefix in a confusing way after Unicode normalization.
				StringPrefixFull("fo").
				NewValue(),
		},
		{
			Input:     cty.StringVal("foo-123"),
			Prefix:    cty.StringVal("foo-"),
			WantValue: cty.StringVal("foo-123"),
		},
		{
			Input:     cty.NullVal(cty.String),
			Prefix:    cty.StringVal("foo-"),
			WantValue: cty.NullVal(cty.String),
		},
		{
			Input:     cty.StringVal("hello"),
			Prefix:    cty.StringVal("foo-"),
			WantError: `assumption was not upheld`,
		},
		{
			Input:     cty.StringVal("foo-123"),
			Prefix:    cty.UnknownVal(cty.String),
			WantValue: cty.UnknownVal(cty.String), // representing that we can't check the assumption yet
		},
	}

	for _, test := range tests {
		t.Run(test.Input.GoString(), func(t *testing.T) {
			gotVal, gotErr := AssumeStringPrefix(test.Input, test.Prefix)

			if wantErr := test.WantError; wantErr != "" {
				if gotErr == nil {
					t.Fatalf("unexpected success\nwant error: %s\ngot result: %#v", wantErr, gotVal)
				}
				if gotErr := gotErr.Error(); gotErr != wantErr {
					t.Fatalf("unexpected error\ngot:  %s\nwant: %s", gotErr, wantErr)
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
