// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package funcs

import (
	"testing"

	"github.com/zclconf/go-cty/cty"
)

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
