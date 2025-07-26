// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonconfig

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tofu"
	"github.com/zclconf/go-cty/cty"
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
	providerAddr := addrs.NewProvider("host", "namespace", "type")
	resSchema := map[string]providers.Schema{
		"test_type": {
			Version: 0,
			Block: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
		},
	}

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
		"resources": {
			Input: &configs.Config{
				Module: &configs.Module{
					ManagedResources: map[string]*configs.Resource{
						"test_res": {
							Mode: addrs.ManagedResourceMode,
							Name: "test_res",
							Type: "test_type",
							Config: &hclsyntax.Body{
								Attributes: map[string]*hclsyntax.Attribute{},
							},
							Provider: providerAddr,
						},
					},
					DataResources: map[string]*configs.Resource{
						"test_data": {
							Mode: addrs.DataResourceMode,
							Name: "test_data",
							Type: "test_type",
							Config: &hclsyntax.Body{
								Attributes: map[string]*hclsyntax.Attribute{},
							},
							Provider: providerAddr,
						},
					},
					EphemeralResources: map[string]*configs.Resource{
						"test_ephemeral": {
							Mode: addrs.EphemeralResourceMode,
							Name: "test_ephemeral",
							Type: "test_type",
							Config: &hclsyntax.Body{
								Attributes: map[string]*hclsyntax.Attribute{},
							},
							Provider: providerAddr,
						},
					},
				},
			},
			Schemas: &tofu.Schemas{
				Providers: map[addrs.Provider]providers.ProviderSchema{
					providerAddr: {
						ResourceTypes:      resSchema,
						EphemeralResources: resSchema,
						DataSources:        resSchema,
					},
				},
			},
			Want: module{
				Outputs:     map[string]output{},
				ModuleCalls: map[string]moduleCall{},
				Resources: []resource{
					{
						Address:           "test_type.test_res",
						Mode:              "managed",
						Type:              "test_type",
						Name:              "test_res",
						ProviderConfigKey: "test",
						SchemaVersion:     ptrTo[uint64](0),
						Provisioners:      nil,
						Expressions:       make(map[string]any),
					},
					{
						Address:           "data.test_type.test_data",
						Mode:              "data",
						Type:              "test_type",
						Name:              "test_data",
						ProviderConfigKey: "test",
						SchemaVersion:     ptrTo[uint64](0),
						Provisioners:      nil,
						Expressions:       make(map[string]any),
					},
					{
						Address:           "ephemeral.test_type.test_ephemeral",
						Mode:              "ephemeral",
						Type:              "test_type",
						Name:              "test_ephemeral",
						ProviderConfigKey: "test",
						SchemaVersion:     ptrTo[uint64](0),
						Provisioners:      nil,
						Expressions:       make(map[string]any),
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

// ptrTo is a helper to compensate for the fact that Go doesn't allow
// using the '&' operator unless the operand is directly addressable.
//
// Instead then, this function returns a pointer to a copy of the given
// value.
func ptrTo[T any](v T) *T {
	return &v
}
