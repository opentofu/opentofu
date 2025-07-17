// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonconfig

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/tofu"
)

func TestFindSourceProviderConfig(t *testing.T) {
	tests := []struct {
		StartKey    string
		FullName    string
		ProviderMap map[string]providerConfig
		Want        string
	}{
		{
			StartKey:    "null",
			FullName:    "hashicorp/null",
			ProviderMap: map[string]providerConfig{},
			Want:        "",
		},
		{
			StartKey: "null",
			FullName: "hashicorp/null",
			ProviderMap: map[string]providerConfig{
				"null": {
					Name:          "null",
					FullName:      "hashicorp/null",
					ModuleAddress: "",
				},
			},
			Want: "null",
		},
		{
			StartKey: "null2",
			FullName: "hashicorp/null",
			ProviderMap: map[string]providerConfig{
				"null": {
					Name:          "null",
					FullName:      "hashicorp/null",
					ModuleAddress: "",
				},
			},
			Want: "",
		},
		{
			StartKey: "null",
			FullName: "hashicorp2/null",
			ProviderMap: map[string]providerConfig{
				"null": {
					Name:          "null",
					FullName:      "hashicorp/null",
					ModuleAddress: "",
				},
			},
			Want: "",
		},
		{
			StartKey: "module.a:null",
			FullName: "hashicorp/null",
			ProviderMap: map[string]providerConfig{
				"null": {
					Name:          "null",
					FullName:      "hashicorp/null",
					ModuleAddress: "",
				},
				"module.a:null": {
					Name:          "module.a:null",
					FullName:      "hashicorp/null",
					ModuleAddress: "module.a",
					parentKey:     "null",
				},
			},
			Want: "null",
		},
		{
			StartKey: "module.a:null",
			FullName: "hashicorp2/null",
			ProviderMap: map[string]providerConfig{
				"null": {
					Name:          "null",
					FullName:      "hashicorp/null",
					ModuleAddress: "",
				},
				"module.a:null": {
					Name:          "module.a:null",
					FullName:      "hashicorp2/null",
					ModuleAddress: "module.a",
					parentKey:     "null",
				},
			},
			Want: "module.a:null",
		},
	}

	for _, test := range tests {
		got := findSourceProviderKey(test.StartKey, test.FullName, test.ProviderMap)
		if got != test.Want {
			t.Errorf("wrong result:\nGot: %#v\nWant: %#v\n", got, test.Want)
		}
	}
}

func TestMarshalModule(t *testing.T) {
	emptySchemas := &tofu.Schemas{}

	tests := map[string]struct {
		Input   *configs.Config
		Schemas *tofu.Schemas
		Want    module
	}{
		"empty": {
			Input: &configs.Config{
				Module: &configs.Module{},
			},
			Schemas: emptySchemas,
			Want: module{
				Outputs:     map[string]output{},
				ModuleCalls: map[string]moduleCall{},
			},
		},
		"variable, minimal": {
			Input: &configs.Config{
				Module: &configs.Module{
					Variables: map[string]*configs.Variable{
						"example": &configs.Variable{
							Name: "example",
						},
					},
				},
			},
			Schemas: emptySchemas,
			Want: module{
				Outputs:     map[string]output{},
				ModuleCalls: map[string]moduleCall{},
				Variables: variables{
					"example": {
						Required: true,
					},
				},
			},
		},
		"variable, elaborate": {
			Input: &configs.Config{
				Module: &configs.Module{
					Variables: map[string]*configs.Variable{
						"example": {
							Name:           "example",
							Description:    "description",
							Deprecated:     "deprecation message",
							Sensitive:      true,
							ConstraintType: cty.String,
							Type:           cty.String, // similar to ConstraintType; unfortunate historical quirk
							Default:        cty.StringVal("hello"),
						},
					},
				},
			},
			Schemas: emptySchemas,
			Want: module{
				Outputs:     map[string]output{},
				ModuleCalls: map[string]moduleCall{},
				Variables: variables{
					"example": {
						Type:        json.RawMessage(`"string"`),
						Default:     json.RawMessage(`"hello"`),
						Required:    false,
						Description: "description",
						Deprecated:  "deprecation message",
						Sensitive:   true,
					},
				},
			},
		},
		"variable, collection type": {
			Input: &configs.Config{
				Module: &configs.Module{
					Variables: map[string]*configs.Variable{
						"example": {
							Name:           "example",
							ConstraintType: cty.List(cty.String),
							Type:           cty.List(cty.String), // similar to ConstraintType; unfortunate historical quirk
						},
					},
				},
			},
			Schemas: emptySchemas,
			Want: module{
				Outputs:     map[string]output{},
				ModuleCalls: map[string]moduleCall{},
				Variables: variables{
					"example": {
						// The following is how cty serializes collection types
						// as JSON: a two-element array where the first is
						// the kind of collection and the second is the
						// element type.
						Type:     json.RawMessage(`["list","string"]`),
						Required: true,
					},
				},
			},
		},
		"variable, object type": {
			Input: &configs.Config{
				Module: &configs.Module{
					Variables: map[string]*configs.Variable{
						"example": {
							Name: "example",
							ConstraintType: cty.ObjectWithOptionalAttrs(map[string]cty.Type{
								"foo": cty.String,
								"bar": cty.String,
							}, []string{"bar"}),
							Type: cty.Object(map[string]cty.Type{
								"foo": cty.String,
							}),
						},
					},
				},
			},
			Schemas: emptySchemas,
			Want: module{
				Outputs:     map[string]output{},
				ModuleCalls: map[string]moduleCall{},
				Variables: variables{
					"example": {
						// The following is how cty serializes structural types
						// as JSON: a two- or three-element array where the
						// first is the kind of structure and the second is the
						// kind-specific structure description, which in
						// this case is a JSON object mapping attribute names
						// to their types. For object types in particular,
						// when at least one optional attribute is included
						// the array has a third element listing the names
						// of the optional attributes.
						Type:     json.RawMessage(`["object",{"bar":"string","foo":"string"},["bar"]]`),
						Required: true,
					},
				},
			},
		},
		// TODO: More test cases covering things other than input variables.
		// (For now the other details are mainly tested in package command,
		// as part of the tests for "tofu show".)
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			schemas := test.Schemas

			// We'll make the input a little more realistic by including some
			// of the cyclic pointers that would normally be inserted by the
			// config loader.
			input := *test.Input
			input.Root = &input
			input.Parent = &input

			got, err := marshalModule(&input, schemas, addrs.RootModule.String())
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if diff := cmp.Diff(test.Want, got); diff != "" {
				t.Error("wrong result\n" + diff)
			}
		})
	}
}
