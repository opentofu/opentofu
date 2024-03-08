// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package gohcl_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/gohcl"
	"github.com/zclconf/go-cty/cty"
)

var data = `
inner "foo" "bar" {
	val = magic.foo.bar
	data = {
		"z" = nested.value
	}
}
`

type InnerBlock struct {
	Type  string            `hcl:"type,label"`
	Name  string            `hcl:"name,label"`
	Value string            `hcl:"val"`
	Data  map[string]string `hcl:"data"`
}

type OuterBlock struct {
	Contents InnerBlock `hcl:"inner,block"`
}

func Test(t *testing.T) {

	println("> Parse HCL")
	file, diags := hclsyntax.ParseConfig([]byte(data), "INLINE", hcl.Pos{Byte: 0, Line: 1, Column: 1})

	println(diags.Error())

	ob := &OuterBlock{}

	println()
	println("> Detect Variables")
	vars, diags := gohcl.VariablesInBody(file.Body, ob)
	println(diags.Error())
	for _, v := range vars {
		ident := ""
		for _, p := range v {
			if root, ok := p.(hcl.TraverseRoot); ok {
				ident += root.Name
			}
			if attr, ok := p.(hcl.TraverseAttr); ok {
				ident += "." + attr.Name
			}
		}
		println("Required: " + ident)
	}

	println()
	println("> Decode Body")

	ctx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"magic": cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"bar": cty.StringVal("BAR IS BEST BAR"),
				}),
			}),
			"nested": cty.ObjectVal(map[string]cty.Value{
				"value": cty.StringVal("ZISHERE"),
			}),
		},
	}

	diags = gohcl.DecodeBody(file.Body, ctx, ob)
	println(diags.Error())

	fmt.Printf("%#v\n", ob)
}
