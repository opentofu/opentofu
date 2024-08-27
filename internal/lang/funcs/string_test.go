// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package funcs

import (
	"fmt"
	"testing"

	"github.com/terramate-io/opentofulib/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

func TestReplace(t *testing.T) {
	tests := []struct {
		String  cty.Value
		Substr  cty.Value
		Replace cty.Value
		Want    cty.Value
		Err     bool
	}{
		{ // Regular search and replace
			cty.StringVal("hello"),
			cty.StringVal("hel"),
			cty.StringVal("bel"),
			cty.StringVal("bello"),
			false,
		},
		{ // Search string doesn't match
			cty.StringVal("hello"),
			cty.StringVal("nope"),
			cty.StringVal("bel"),
			cty.StringVal("hello"),
			false,
		},
		{ // Regular expression
			cty.StringVal("hello"),
			cty.StringVal("/l/"),
			cty.StringVal("L"),
			cty.StringVal("heLLo"),
			false,
		},
		{
			cty.StringVal("helo"),
			cty.StringVal("/(l)/"),
			cty.StringVal("$1$1"),
			cty.StringVal("hello"),
			false,
		},
		{ // Bad regexp
			cty.StringVal("hello"),
			cty.StringVal("/(l/"),
			cty.StringVal("$1$1"),
			cty.UnknownVal(cty.String),
			true,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("replace(%#v, %#v, %#v)", test.String, test.Substr, test.Replace), func(t *testing.T) {
			got, err := Replace(test.String, test.Substr, test.Replace)

			if test.Err {
				if err == nil {
					t.Fatal("succeeded; want error")
				}
				return
			} else if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if !got.RawEquals(test.Want) {
				t.Errorf("wrong result\ngot:  %#v\nwant: %#v", got, test.Want)
			}
		})
	}
}

func TestStrContains(t *testing.T) {
	tests := []struct {
		String cty.Value
		Substr cty.Value
		Want   cty.Value
		Err    bool
	}{
		{
			cty.StringVal("hello"),
			cty.StringVal("hel"),
			cty.BoolVal(true),
			false,
		},
		{
			cty.StringVal("hello"),
			cty.StringVal("lo"),
			cty.BoolVal(true),
			false,
		},
		{
			cty.StringVal("hello1"),
			cty.StringVal("1"),
			cty.BoolVal(true),
			false,
		},
		{
			cty.StringVal("hello1"),
			cty.StringVal("heo"),
			cty.BoolVal(false),
			false,
		},
		{
			cty.StringVal("hello1"),
			cty.NumberIntVal(1),
			cty.UnknownVal(cty.Bool),
			true,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("includes(%#v, %#v)", test.String, test.Substr), func(t *testing.T) {
			got, err := StrContains(test.String, test.Substr)

			if test.Err {
				if err == nil {
					t.Fatal("succeeded; want error")
				}
				return
			} else if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if !got.RawEquals(test.Want) {
				t.Errorf("wrong result\ngot:  %#v\nwant: %#v", got, test.Want)
			}
		})
	}
}

func TestStartsWith(t *testing.T) {
	tests := []struct {
		String, Prefix cty.Value
		Want           cty.Value
		WantError      string
	}{
		{
			cty.StringVal("hello world"),
			cty.StringVal("hello"),
			cty.True,
			``,
		},
		{
			cty.StringVal("hey world"),
			cty.StringVal("hello"),
			cty.False,
			``,
		},
		{
			cty.StringVal(""),
			cty.StringVal(""),
			cty.True,
			``,
		},
		{
			cty.StringVal("a"),
			cty.StringVal(""),
			cty.True,
			``,
		},
		{
			cty.StringVal(""),
			cty.StringVal("a"),
			cty.False,
			``,
		},
		{
			cty.UnknownVal(cty.String),
			cty.StringVal("a"),
			cty.UnknownVal(cty.Bool).RefineNotNull(),
			``,
		},
		{
			cty.UnknownVal(cty.String),
			cty.StringVal(""),
			cty.True,
			``,
		},
		{
			cty.UnknownVal(cty.String).Refine().StringPrefix("https:").NewValue(),
			cty.StringVal(""),
			cty.True,
			``,
		},
		{
			cty.UnknownVal(cty.String).Refine().StringPrefix("https:").NewValue(),
			cty.StringVal("a"),
			cty.False,
			``,
		},
		{
			cty.UnknownVal(cty.String).Refine().StringPrefix("https:").NewValue(),
			cty.StringVal("ht"),
			cty.True,
			``,
		},
		{
			cty.UnknownVal(cty.String).Refine().StringPrefix("https:").NewValue(),
			cty.StringVal("https:"),
			cty.True,
			``,
		},
		{
			cty.UnknownVal(cty.String).Refine().StringPrefix("https:").NewValue(),
			cty.StringVal("https-"),
			cty.False,
			``,
		},
		{
			cty.UnknownVal(cty.String).Refine().StringPrefix("https:").NewValue(),
			cty.StringVal("https://"),
			cty.UnknownVal(cty.Bool).RefineNotNull(),
			``,
		},
		{
			// Unicode combining characters edge-case: we match the prefix
			// in terms of unicode code units rather than grapheme clusters,
			// which is inconsistent with our string processing elsewhere but
			// would be a breaking change to fix that bug now.
			cty.StringVal("\U0001f937\u200d\u2642"), // "Man Shrugging" is encoded as "Person Shrugging" followed by zero-width joiner and then the masculine gender presentation modifier
			cty.StringVal("\U0001f937"),             // Just the "Person Shrugging" character without any modifiers
			cty.True,
			``,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("StartsWith(%#v, %#v)", test.String, test.Prefix), func(t *testing.T) {
			got, err := StartsWithFunc.Call([]cty.Value{test.String, test.Prefix})

			if test.WantError != "" {
				gotErr := fmt.Sprintf("%s", err)
				if gotErr != test.WantError {
					t.Errorf("wrong error\ngot:  %s\nwant: %s", gotErr, test.WantError)
				}
				return
			} else if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if !got.RawEquals(test.Want) {
				t.Errorf(
					"wrong result\nstring: %#v\nprefix: %#v\ngot:    %#v\nwant:   %#v",
					test.String, test.Prefix, got, test.Want,
				)
			}
		})
	}
}

func TestTemplateString(t *testing.T) {
	tests := map[string]struct {
		Content cty.Value
		Vars    cty.Value
		Want    cty.Value
		Err     string
	}{
		"Simple string template": {
			cty.StringVal("Hello, Jodie!"),
			cty.EmptyObjectVal,
			cty.StringVal("Hello, Jodie!"),
			``,
		},
		"String interpolation with variable": {
			cty.StringVal("Hello, ${name}!"),
			cty.MapVal(map[string]cty.Value{
				"name": cty.StringVal("Jodie"),
			}),
			cty.StringVal("Hello, Jodie!"),
			``,
		},
		"Looping through list": {
			cty.StringVal("Items: %{ for x in list ~} ${x} %{ endfor ~}"),
			cty.ObjectVal(map[string]cty.Value{
				"list": cty.ListVal([]cty.Value{
					cty.StringVal("a"),
					cty.StringVal("b"),
					cty.StringVal("c"),
				}),
			}),
			cty.StringVal("Items: a b c "),
			``,
		},
		"Looping through map": {
			cty.StringVal("%{ for key, value in list ~} ${key}:${value} %{ endfor ~}"),
			cty.ObjectVal(map[string]cty.Value{
				"list": cty.ObjectVal(map[string]cty.Value{
					"item1": cty.StringVal("a"),
					"item2": cty.StringVal("b"),
					"item3": cty.StringVal("c"),
				}),
			}),
			cty.StringVal("item1:a item2:b item3:c "),
			``,
		},
		"Invalid template variable name": {
			cty.StringVal("Hello, ${1}!"),
			cty.MapVal(map[string]cty.Value{
				"1": cty.StringVal("Jodie"),
			}),
			cty.NilVal,
			`invalid template variable name "1": must start with a letter, followed by zero or more letters, digits, and underscores`,
		},
		"Variable not present in vars map": {
			cty.StringVal("Hello, ${name}!"),
			cty.EmptyObjectVal,
			cty.NilVal,
			`vars map does not contain key "name"`,
		},
		"Interpolation of a boolean value": {
			cty.StringVal("${val}"),
			cty.ObjectVal(map[string]cty.Value{
				"val": cty.True,
			}),
			cty.True,
			``,
		},
		"Sensitive string template": {
			cty.StringVal("My password is 1234").Mark(marks.Sensitive),
			cty.EmptyObjectVal,
			cty.StringVal("My password is 1234").Mark(marks.Sensitive),
			``,
		},
		"Sensitive template variable": {
			cty.StringVal("My password is ${pass}"),
			cty.ObjectVal(map[string]cty.Value{
				"pass": cty.StringVal("secret").Mark(marks.Sensitive),
			}),
			cty.StringVal("My password is secret").Mark(marks.Sensitive),
			``,
		},
	}

	templateStringFn := MakeTemplateStringFunc(".", func() map[string]function.Function {
		return map[string]function.Function{}
	})

	for _, test := range tests {
		t.Run(fmt.Sprintf("TemplateString(%#v, %#v)", test.Content, test.Vars), func(t *testing.T) {
			got, err := templateStringFn.Call([]cty.Value{test.Content, test.Vars})

			if argErr, ok := err.(function.ArgError); ok {
				if argErr.Index < 0 || argErr.Index > 1 {
					t.Errorf("ArgError index %d is out of range for templatestring (must be 0 or 1)", argErr.Index)
				}
			}

			if err != nil {
				if test.Err == "" {
					t.Fatalf("unexpected error: %s", err)
				} else {
					if got, want := err.Error(), test.Err; got != want {
						t.Errorf("wrong error\ngot:  %s\nwant: %s", got, want)
					}
				}
			} else if test.Err != "" {
				t.Fatal("succeeded; want error")
			} else {
				if !got.RawEquals(test.Want) {
					t.Errorf("wrong result\ngot:  %#v\nwant: %#v", got, test.Want)
				}
			}
		})
	}
}
