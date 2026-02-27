// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonfunction

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2/ext/tryfunc"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

func TestMarshal(t *testing.T) {
	tests := []struct {
		Name    string
		Input   map[string]function.Function
		Want    string
		WantErr string
	}{
		{
			"minimal function",
			map[string]function.Function{
				"fun": function.New(&function.Spec{
					Type: function.StaticReturnType(cty.Bool),
				}),
			},
			`{"format_version":"1.0","function_signatures":{"fun":{"return_type":"bool"}}}`,
			"",
		},
		{
			"function with description",
			map[string]function.Function{
				"fun": function.New(&function.Spec{
					Description: "`timestamp` returns a UTC timestamp string.",
					Type:        function.StaticReturnType(cty.String),
				}),
			},
			"{\"format_version\":\"1.0\",\"function_signatures\":{\"fun\":{\"description\":\"`timestamp` returns a UTC timestamp string.\",\"return_type\":\"string\"}}}",
			"",
		},
		{
			"function with parameters",
			map[string]function.Function{
				"fun": function.New(&function.Spec{
					Params: []function.Parameter{
						{
							Name:        "timestamp",
							Description: "timestamp text",
							Type:        cty.String,
						},
						{
							Name:        "duration",
							Description: "duration text",
							Type:        cty.String,
						},
					},
					Type: function.StaticReturnType(cty.String),
				}),
			},
			`{"format_version":"1.0","function_signatures":{"fun":{"return_type":"string","parameters":[{"name":"timestamp","description":"timestamp text","type":"string"},{"name":"duration","description":"duration text","type":"string"}]}}}`,
			"",
		},
		{
			"function with variadic parameter",
			map[string]function.Function{
				"fun": function.New(&function.Spec{
					VarParam: &function.Parameter{
						Name:             "default",
						Description:      "default description",
						Type:             cty.DynamicPseudoType,
						AllowUnknown:     true,
						AllowDynamicType: true,
						AllowNull:        true,
						AllowMarked:      true,
					},
					Type: function.StaticReturnType(cty.DynamicPseudoType),
				}),
			},
			`{"format_version":"1.0","function_signatures":{"fun":{"return_type":"dynamic","variadic_parameter":{"name":"default","description":"default description","is_nullable":true,"type":"dynamic"}}}}`,
			"",
		},
		{
			"function with list types",
			map[string]function.Function{
				"fun": function.New(&function.Spec{
					Params: []function.Parameter{
						{
							Name: "list",
							Type: cty.List(cty.String),
						},
					},
					Type: function.StaticReturnType(cty.List(cty.String)),
				}),
			},
			`{"format_version":"1.0","function_signatures":{"fun":{"return_type":["list","string"],"parameters":[{"name":"list","type":["list","string"]}]}}}`,
			"",
		},
		{
			"returns diagnostics on failure",
			map[string]function.Function{
				"fun": function.New(&function.Spec{
					Params: []function.Parameter{},
					Type: func(args []cty.Value) (ret cty.Type, err error) {
						return cty.DynamicPseudoType, fmt.Errorf("error")
					},
				}),
			},
			"",
			"Failed to serialize function \"fun\": error",
		},
		{
			"try function marshalled correctly",
			map[string]function.Function{
				"try": tryfunc.TryFunc,
			},
			`{"format_version":"1.0","function_signatures":{"try":{"return_type":"dynamic","variadic_parameter":{"name":"expressions","type":"dynamic"}}}}`,
			"",
		},
		{
			"core::try function marshalled correctly",
			map[string]function.Function{
				"core::try": tryfunc.TryFunc,
			},
			`{"format_version":"1.0","function_signatures":{"core::try":{"return_type":"dynamic","variadic_parameter":{"name":"expressions","type":"dynamic"}}}}`,
			"",
		},
		{
			// This checks that if a provider contains a function named the same as one of the core with custom marshaller, we identify that correctly
			"provider::test::try function marshalled correctly",
			map[string]function.Function{
				"provider::test::try": function.New(&function.Spec{
					Params: []function.Parameter{
						{
							Name: "list",
							Type: cty.List(cty.String),
						},
					},
					Type: function.StaticReturnType(cty.List(cty.String)),
				}),
			},
			`{"format_version":"1.0","function_signatures":{"provider::test::try":{"return_type":["list","string"],"parameters":[{"name":"list","type":["list","string"]}]}}}`,
			"",
		},
		{
			"can function marshalled correctly",
			map[string]function.Function{
				"can": tryfunc.CanFunc,
			},
			`{"format_version":"1.0","function_signatures":{"can":{"return_type":"bool","parameters":[{"name":"expression","type":"dynamic"}]}}}`,
			"",
		},
		{
			"core::can function marshalled correctly",
			map[string]function.Function{
				"core::can": tryfunc.CanFunc,
			},
			`{"format_version":"1.0","function_signatures":{"core::can":{"return_type":"bool","parameters":[{"name":"expression","type":"dynamic"}]}}}`,
			"",
		},
		{
			// This checks that if a provider contains a function named the same as one of the core with custom marshaller, we identify that correctly
			"provider::test::can function marshalled correctly",
			map[string]function.Function{
				"provider::test::can": function.New(&function.Spec{
					Params: []function.Parameter{
						{
							Name: "list",
							Type: cty.List(cty.String),
						},
					},
					Type: function.StaticReturnType(cty.List(cty.String)),
				}),
			},
			`{"format_version":"1.0","function_signatures":{"provider::test::can":{"return_type":["list","string"],"parameters":[{"name":"list","type":["list","string"]}]}}}`,
			"",
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("%d-%s", i, test.Name), func(t *testing.T) {
			got, diags := Marshal(test.Input)
			if test.WantErr != "" {
				if !diags.HasErrors() {
					t.Fatal("expected error, got none")
				}
				if diags.Err().Error() != test.WantErr {
					t.Fatalf("expected error %q, got %q", test.WantErr, diags.Err())
				}
			} else {
				if diags.HasErrors() {
					t.Fatal(diags)
				}

				if diff := cmp.Diff(test.Want, string(got), ctydebug.CmpOptions); diff != "" {
					t.Fatalf("mismatch of function signature: %s", diff)
				}
			}
		})
	}
}
