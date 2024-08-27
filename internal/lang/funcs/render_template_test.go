// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package funcs

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/terramate-io/opentofulib/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

func TestRenderTemplate(t *testing.T) {
	tests := map[string]struct {
		Expr hcl.Expression
		Vars cty.Value
		Want cty.Value
		Err  string
	}{
		"String interpolation with variable": {
			hcl.StaticExpr(cty.StringVal("Hello, ${name}!"), hcl.Range{}),
			cty.MapVal(map[string]cty.Value{
				"name": cty.StringVal("Jodie"),
			}),
			cty.StringVal("Hello, ${name}!"),
			``,
		},
		"Looping through list": {
			hcl.StaticExpr(cty.StringVal("Items: %{ for x in list ~} ${x} %{ endfor ~}"), hcl.Range{}),
			cty.ObjectVal(map[string]cty.Value{
				"list": cty.ListVal([]cty.Value{
					cty.StringVal("a"),
					cty.StringVal("b"),
					cty.StringVal("c"),
				}),
			}),
			cty.StringVal("Items: %{ for x in list ~} ${x} %{ endfor ~}"),
			``,
		},
		"Looping through map": {
			hcl.StaticExpr(cty.StringVal("%{ for key, value in list ~} ${key}:${value} %{ endfor ~}"), hcl.Range{}),
			cty.ObjectVal(map[string]cty.Value{
				"list": cty.ObjectVal(map[string]cty.Value{
					"item1": cty.StringVal("a"),
					"item2": cty.StringVal("b"),
					"item3": cty.StringVal("c"),
				}),
			}),
			cty.StringVal("%{ for key, value in list ~} ${key}:${value} %{ endfor ~}"),
			``,
		},
		"Invalid template variable name": {
			hcl.StaticExpr(cty.StringVal("Hello, ${1}!"), hcl.Range{}),
			cty.MapVal(map[string]cty.Value{
				"1": cty.StringVal("Jodie"),
			}),
			cty.NilVal,
			`invalid template variable name "1": must start with a letter, followed by zero or more letters, digits, and underscores`,
		},
		"Interpolation of a boolean value": {
			hcl.StaticExpr(cty.StringVal("${val}"), hcl.Range{}),
			cty.ObjectVal(map[string]cty.Value{
				"val": cty.True,
			}),
			cty.StringVal("${val}"),
			``,
		},
		"Sensitive string template": {
			hcl.StaticExpr(cty.StringVal("My password is 1234").Mark(marks.Sensitive), hcl.Range{}),
			cty.EmptyObjectVal,
			cty.StringVal("My password is 1234").Mark(marks.Sensitive),
			``,
		},
		"Sensitive template variable": {
			hcl.StaticExpr(cty.StringVal("My password is ${pass}"), hcl.Range{}),
			cty.ObjectVal(map[string]cty.Value{
				"pass": cty.StringVal("secret").Mark(marks.Sensitive),
			}),
			cty.StringVal("My password is ${pass}"),
			``,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {

			got, err := renderTemplate(test.Expr, test.Vars, map[string]function.Function{})

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
