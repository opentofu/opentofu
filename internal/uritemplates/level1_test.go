// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package uritemplates

import (
	"testing"
)

func TestExpandLevel1(t *testing.T) {
	tests := []struct {
		input   string
		vars    map[string]string
		want    string
		wantErr string
	}{
		{
			``,
			nil,
			``,
			``,
		},
		{
			`foo`,
			nil,
			`foo`,
			``,
		},
		{
			// This is example is from RFC 6570 section 1.2
			`{var}`,
			map[string]string{
				"var": "value",
			},
			`value`,
			``,
		},
		{
			// This is example is from RFC 6570 section 1.2
			`{hello}`,
			map[string]string{
				"hello": "Hello World!",
			},
			`Hello%20World%21`,
			``,
		},
		{
			// This is example is from RFC 6570 section 1.2
			`beep/{with_slash}/boop`,
			map[string]string{
				"with_slash": "foo/bar",
			},
			`beep/foo%2fbar/boop`,
			``,
		},
		{
			// This is example is from RFC 6570 section 1.2
			`beep/{with_question}/boop`,
			map[string]string{
				"with_question": "foo?bar",
			},
			`beep/foo%3fbar/boop`,
			``,
		},
		{
			// This is an example for something that maps a provider source address into a URI
			`https://example.com/{hostname}/{namespace}/provider-{type}.zip`,
			map[string]string{
				"hostname":  "example.net",
				"namespace": "ほげ",
				"type":      "ふが",
			},
			// The URI template spec requires non-ASCII characters to be percent-encoded.
			`https://example.com/example.net/%e3%81%bb%e3%81%92/provider-%e3%81%b5%e3%81%8c.zip`,
			``,
		},
		{
			`foo{bar}`,
			map[string]string{
				"bar": "baz",
			},
			`foobaz`,
			``,
		},
		{
			`hello_{undefined}_world`,
			nil,
			`hello__world`, // undefined variable expands to the emptys tring
			``,
		},
		{
			`{oops`,
			nil,
			``,
			`unclosed URI template expression`,
		},
		{
			`whoopsy{daisy`,
			nil,
			`whoopsy`,
			`unclosed URI template expression`,
		},
		{
			`uh{oh}this{isnt valid`,
			map[string]string{
				"oh": "uh",
			},
			`uhuhthis`,
			`unclosed URI template expression`,
		},
		{
			`{bleep%2fbloop}`,
			map[string]string{
				// percent-encoded sequences in variable names are defined as
				// a literal part of a variable name in the spec, and must not
				// be decoded before lookup.
				"bleep%2fbloop":  "correct",
				"bleep\x2fbloop": "incorrect",
			},
			`correct`,
			``,
		},
		{
			`%2f`,
			nil,
			`%2f`,
			``,
		},
		{
			`{+bar}`,
			nil,
			``,
			`level 2 template expression operator '+' not allowed; only level 1 templates are supported`,
		},
		{
			`{#bar}`,
			nil,
			``,
			`level 2 template expression operator '#' not allowed; only level 1 templates are supported`,
		},
		{
			`{.bar}`,
			nil,
			``,
			`level 3 template expression operator '.' not allowed; only level 1 templates are supported`,
		},
		{
			`{/bar}`,
			nil,
			``,
			`level 3 template expression operator '/' not allowed; only level 1 templates are supported`,
		},
		{
			`{;bar}`,
			nil,
			``,
			`level 3 template expression operator ';' not allowed; only level 1 templates are supported`,
		},
		{
			`{?bar}`,
			nil,
			``,
			`level 3 template expression operator '?' not allowed; only level 1 templates are supported`,
		},
		{
			`{&bar}`,
			nil,
			``,
			`level 3 template expression operator '&' not allowed; only level 1 templates are supported`,
		},
		{
			`{=bar}`,
			nil,
			``,
			`reserved template expression operator '=' not allowed`,
		},
		{
			`{,bar}`,
			nil,
			``,
			`reserved template expression operator ',' not allowed`,
		},
		{
			`{!bar}`,
			nil,
			``,
			`reserved template expression operator '!' not allowed`,
		},
		{
			`{@bar}`,
			nil,
			``,
			`reserved template expression operator '@' not allowed`,
		},
		{
			`{|bar}`,
			nil,
			``,
			`reserved template expression operator '|' not allowed`,
		},
		{
			`{bar:12}`,
			nil,
			``,
			`level 4 modifier ':' not allowed`,
		},
		{
			`{bar*}`,
			nil,
			``,
			`level 4 modifier '*' not allowed`,
		},
		{
			`%no`,
			nil,
			``,
			`invalid percent-encoded character`,
		},
		{
			`%`,
			nil,
			``,
			`invalid percent-encoded character`,
		},
		{
			`{bleep%bloop}`,
			nil,
			``,
			`invalid percent-encoded character`,
		},
		{
			`{bleep%}`,
			nil,
			``,
			`invalid percent-encoded character`,
		},
		{
			`{bleep bloop}`,
			nil,
			``,
			`invalid symbol ' ' in variable name`,
		},
		{
			`{ bleepbloop}`,
			nil,
			``,
			`invalid symbol ' ' in variable name`,
		},
		{
			`{bleepbloop }`,
			nil,
			``,
			`invalid symbol ' ' in variable name`,
		},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, gotErr := ExpandLevel1(test.input, test.vars)

			if test.wantErr != "" {
				if gotErr == nil {
					t.Errorf("unexpected success\n  want error: %s", test.wantErr)
				} else if gotErrStr, wantErrStr := gotErr.Error(), test.wantErr; gotErrStr != wantErrStr {
					t.Errorf("wrong error\ngot:  %s\nwant: %s", gotErrStr, wantErrStr)
				}
			} else if gotErr != nil {
				t.Errorf("unexpected error: %s", gotErr)
			}

			if got != test.want {
				t.Errorf("wrong result\ngot:  %s\nwant: %s", got, test.want)
			}
		})
	}
}

func TestValidateLevel1(t *testing.T) {
	tests := []struct {
		input   string
		wantErr string
	}{
		{
			``,
			``,
		},
		{
			`foo`,
			``,
		},
		{
			`foo{bar}`,
			``,
		},
		{
			`foo{bar}baz`,
			``,
		},
		{
			`{bar}baz`,
			``,
		},
		{
			`{bar}`,
			``,
		},
		{
			`{oops`,
			`unclosed URI template expression`,
		},
		{
			`whoopsy{daisy`,
			`unclosed URI template expression`,
		},
		{
			`uh{oh}this{isnt valid`,
			`unclosed URI template expression`,
		},
		{
			`{bleep%2fbloop}`,
			``,
		},
		{
			`%2f`,
			``,
		},
		{
			`{+bar}`,
			`level 2 template expression operator '+' not allowed; only level 1 templates are supported`,
		},
		{
			`{#bar}`,
			`level 2 template expression operator '#' not allowed; only level 1 templates are supported`,
		},
		{
			`{.bar}`,
			`level 3 template expression operator '.' not allowed; only level 1 templates are supported`,
		},
		{
			`{/bar}`,
			`level 3 template expression operator '/' not allowed; only level 1 templates are supported`,
		},
		{
			`{;bar}`,
			`level 3 template expression operator ';' not allowed; only level 1 templates are supported`,
		},
		{
			`{?bar}`,
			`level 3 template expression operator '?' not allowed; only level 1 templates are supported`,
		},
		{
			`{&bar}`,
			`level 3 template expression operator '&' not allowed; only level 1 templates are supported`,
		},
		{
			`{=bar}`,
			`reserved template expression operator '=' not allowed`,
		},
		{
			`{,bar}`,
			`reserved template expression operator ',' not allowed`,
		},
		{
			`{!bar}`,
			`reserved template expression operator '!' not allowed`,
		},
		{
			`{@bar}`,
			`reserved template expression operator '@' not allowed`,
		},
		{
			`{|bar}`,
			`reserved template expression operator '|' not allowed`,
		},
		{
			`{bar:12}`,
			`level 4 modifier ':' not allowed`,
		},
		{
			`{bar*}`,
			`level 4 modifier '*' not allowed`,
		},
		{
			`%no`,
			`invalid percent-encoded character`,
		},
		{
			`%`,
			`invalid percent-encoded character`,
		},
		{
			`{bleep%bloop}`,
			`invalid percent-encoded character`,
		},
		{
			`{bleep%}`,
			`invalid percent-encoded character`,
		},
		{
			`{bleep bloop}`,
			`invalid symbol ' ' in variable name`,
		},
		{
			`{ bleepbloop}`,
			`invalid symbol ' ' in variable name`,
		},
		{
			`{bleepbloop }`,
			`invalid symbol ' ' in variable name`,
		},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			gotErr := ValidateLevel1(test.input)

			if test.wantErr != "" {
				if gotErr == nil {
					t.Fatalf("unexpected success\n  want error: %s", test.wantErr)
				}
				if got, want := gotErr.Error(), test.wantErr; got != want {
					t.Fatalf("wrong error\n  got:  %s\n  want: %s", got, want)
				}
				return
			}

			if gotErr != nil {
				t.Fatalf("unexpected error: %s", gotErr)
			}
		})
	}
}
