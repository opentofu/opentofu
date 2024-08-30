// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonprovider

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
)

func TestMarshalReturnType(t *testing.T) {
	type testcase struct {
		Arg      cty.Type
		Expected any
	}

	tests := map[string]testcase{
		"string": {
			Arg:      cty.String,
			Expected: "string",
		},
		"number": {
			Arg:      cty.Number,
			Expected: "number",
		},
		"bool": {
			Arg:      cty.Bool,
			Expected: "bool",
		},
		"object": {
			Arg: cty.Object(map[string]cty.Type{"number_type": cty.Number}),
			Expected: []any{
				string("object"),
				map[string]cty.Type{"number_type": cty.Number},
			},
		},
		"map": {
			Arg: cty.Map(cty.String),
			Expected: []any{
				string("map"),
				cty.String,
			},
		},
		"list": {
			Arg: cty.List(cty.Bool),
			Expected: []any{
				string("list"),
				cty.Bool,
			},
		},
		"set": {
			Arg: cty.Set(cty.Number),
			Expected: []any{
				string("set"),
				cty.Number,
			},
		},
		"tuple": {
			Arg: cty.Tuple([]cty.Type{cty.String}),
			Expected: []any{
				string("tuple"),
				[]any{cty.String},
			},
		},
	}

	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			actual := marshalReturnType(tc.Arg)

			// to avoid the nightmare of comparing cty primitive types we can marshal them to json and compare that
			actualJSON, _ := json.Marshal(actual)
			expectedJSON, _ := json.Marshal(tc.Expected)
			if !cmp.Equal(actualJSON, expectedJSON) {
				t.Fatalf("values don't match:\n %v\n", cmp.Diff(string(actualJSON), string(expectedJSON)))
			}
		})
	}
}

func TestMarshalParameter(t *testing.T) {
	// used so can make a pointer to it
	trueBoolVal := true

	type testcase struct {
		Arg      providers.FunctionParameterSpec
		Expected FunctionParam
	}

	tests := map[string]testcase{
		"basic": {
			Arg: providers.FunctionParameterSpec{
				Description: "basic string func",
				Type:        cty.String,
			},
			Expected: FunctionParam{
				Description: "basic string func",
				Type:        cty.String,
			},
		},
		"nullable": {
			Arg: providers.FunctionParameterSpec{
				Description:    "nullable number func",
				Type:           cty.Number,
				AllowNullValue: trueBoolVal,
			},
			Expected: FunctionParam{
				Description: "nullable number func",
				Type:        cty.Number,
				IsNullable:  &trueBoolVal,
			},
		},
	}

	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			actual := marshalParameter(tc.Arg)

			// to avoid the nightmare of comparing cty primitive types we can marshal them to json and compare that
			actualJSON, _ := json.Marshal(actual)
			expectedJSON, _ := json.Marshal(tc.Expected)
			if !cmp.Equal(actualJSON, expectedJSON) {
				t.Fatalf("values don't match:\n %v\n", cmp.Diff(string(actualJSON), string(expectedJSON)))
			}
		})
	}
}

func TestMarshalParameters(t *testing.T) {
	type testcase struct {
		Arg      []providers.FunctionParameterSpec
		Expected []FunctionParam
	}

	tests := map[string]testcase{
		"basic": {
			Arg: []providers.FunctionParameterSpec{{
				Description: "basic string func",
				Type:        cty.String,
			}},
			Expected: []FunctionParam{{
				Description: "basic string func",
				Type:        cty.String,
			}},
		},
	}

	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			actual := marshalParameters(tc.Arg)

			// to avoid the nightmare of comparing cty primitive types we can marshal them to json and compare that
			actualJSON, _ := json.Marshal(actual)
			expectedJSON, _ := json.Marshal(tc.Expected)
			if !cmp.Equal(actualJSON, expectedJSON) {
				t.Fatalf("values don't match:\n %v\n", cmp.Diff(string(actualJSON), string(expectedJSON)))
			}
		})
	}
}

func TestMarshalFunction(t *testing.T) {
	type testcase struct {
		Arg      providers.FunctionSpec
		Expected Function
	}

	tests := map[string]testcase{
		"basic": {
			Arg: providers.FunctionSpec{
				Description: "basic string func",
				Return:      cty.String,
			},
			Expected: Function{
				Description: "basic string func",
				ReturnType:  cty.String,
			},
		},
		"variadic": {
			Arg: providers.FunctionSpec{
				Description: "basic string func",
				Return:      cty.String,
				VariadicParameter: &providers.FunctionParameterSpec{
					Description: "basic string func",
					Type:        cty.String,
				},
			},
			Expected: Function{
				Description: "basic string func",
				ReturnType:  cty.String,
				VariadicParameter: &FunctionParam{
					Description: "basic string func",
					Type:        cty.String,
				},
			},
		},
	}

	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			actual := marshalFunction(tc.Arg)

			// to avoid the nightmare of comparing cty primitive types we can marshal them to json and compare that
			actualJSON, _ := json.Marshal(actual)
			expectedJSON, _ := json.Marshal(tc.Expected)
			if !cmp.Equal(actualJSON, expectedJSON) {
				t.Fatalf("values don't match:\n %v\n", cmp.Diff(string(actualJSON), string(expectedJSON)))
			}
		})
	}
}

func TestMarshalFunctions(t *testing.T) {
	type testcase struct {
		Arg      map[string]providers.FunctionSpec
		Expected map[string]Function
	}

	tests := map[string]testcase{
		"basic": {
			Arg: map[string]providers.FunctionSpec{"basic_func": {
				Description: "basic string func",
				Return:      cty.String,
			}},
			Expected: map[string]Function{"basic_func": {
				Description: "basic string func",
				ReturnType:  cty.String,
			}},
		},
	}

	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			actual := marshalFunctions(tc.Arg)

			// to avoid the nightmare of comparing cty primitive types we can marshal them to json and compare that
			actualJSON, _ := json.Marshal(actual)
			expectedJSON, _ := json.Marshal(tc.Expected)
			if !cmp.Equal(actualJSON, expectedJSON) {
				t.Fatalf("values don't match:\n %v\n", cmp.Diff(string(actualJSON), string(expectedJSON)))
			}
		})
	}
}
