// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tf

import (
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2/hclwrite"

	"github.com/zclconf/go-cty/cty"
)

type test struct {
	name          string
	arg           cty.Value
	want          cty.Value
	expectedError error
}

// formatHCLWithoutWhitespaces removes all whitespaces from the HCL string
// This will not result in a valid HCL string, but it will allow us to compare the result without worrying about whitespaces
func formatHCLWithoutWhitespaces(val cty.Value) string {
	if val.IsNull() || !val.Type().Equals(cty.String) {
		panic("formatHCLWithoutWhitespaces only works with string values")
	}
	f := string(hclwrite.Format([]byte(val.AsString())))
	f = strings.ReplaceAll(f, " ", "")
	f = strings.ReplaceAll(f, "\n", "")
	f = strings.ReplaceAll(f, "\t", "")
	return f
}

func TestDecodeTFVarsFunc(t *testing.T) {
	tests := []test{
		{
			name: "basic test",
			arg: cty.StringVal(`
				test = 2
			`),
			want: cty.ObjectVal(map[string]cty.Value{
				"test": cty.NumberIntVal(2),
			}),
			expectedError: nil,
		},
		{
			name: "object basic test",
			arg: cty.StringVal(`
				test = {
					k = "v"
				}
			`),
			want: cty.ObjectVal(map[string]cty.Value{
				"test": cty.ObjectVal(map[string]cty.Value{
					"k": cty.StringVal("v"),
				}),
			}),
			expectedError: nil,
		},
		{
			name: "list basic test",
			arg: cty.StringVal(`
				test = [
					"i1",
					"i2",
					3
				]
			`),
			want: cty.ObjectVal(map[string]cty.Value{
				"test": cty.TupleVal([]cty.Value{
					cty.StringVal("i1"),
					cty.StringVal("i2"),
					cty.NumberIntVal(3),
				}),
			}),
		},
		{
			name: "list of objects",
			arg: cty.StringVal(`
				test = [
					{
						o1k1 = "o1v1"
					},
					{
						o2k1 = "o2v1"
						o2k2 = {
							o3k1 = "o3v1"
						}
					}
				]
			`),
			want: cty.ObjectVal(map[string]cty.Value{
				"test": cty.TupleVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"o1k1": cty.StringVal("o1v1"),
					}),
					cty.ObjectVal(map[string]cty.Value{
						"o2k1": cty.StringVal("o2v1"),
						"o2k2": cty.ObjectVal(map[string]cty.Value{
							"o3k1": cty.StringVal("o3v1"),
						}),
					}),
				}),
			}),
		},
		{
			name: "empty object",
			arg:  cty.StringVal(""),
			want: cty.ObjectVal(map[string]cty.Value{}),
		},
		{
			name:          "invalid content",
			arg:           cty.StringVal("test"), // not a valid HCL
			want:          cty.NullVal(cty.DynamicPseudoType),
			expectedError: errFailedToDecode,
		},
		{
			name:          "invalid content 2",
			arg:           cty.StringVal("{}"), // not a valid HCL
			want:          cty.NullVal(cty.DynamicPseudoType),
			expectedError: errFailedToDecode,
		},
		{
			name:          "invalid content 3",
			arg:           cty.StringVal("\"5*5\": 3"), // not a valid HCL
			want:          cty.NullVal(cty.DynamicPseudoType),
			expectedError: errFailedToDecode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decodeTFVars := &decodeTFVarsFunc{}
			got, err := decodeTFVars.Call([]cty.Value{tt.arg})
			if !errors.Is(err, tt.expectedError) {
				t.Errorf("Call() error = %v, expected %v", err, tt.expectedError)
			}
			if got.NotEqual(tt.want).True() {
				t.Errorf("Call() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEncodeTFVarsFunc(t *testing.T) {
	tests := []test{
		{
			name: "empty object",
			arg:  cty.ObjectVal(map[string]cty.Value{}),
			want: cty.StringVal(""),
		},
		{
			name: "basic test",
			arg: cty.ObjectVal(map[string]cty.Value{
				"test": cty.NumberIntVal(2),
			}),
			want: cty.StringVal(`
				test = 2
			`),
			expectedError: nil,
		},
		{
			name: "object basic test",
			arg: cty.ObjectVal(map[string]cty.Value{
				"test": cty.ObjectVal(map[string]cty.Value{
					"k": cty.StringVal("v"),
				}),
			}),
			want: cty.StringVal(`
				test = {
					k = "v"
				}
			`),
			expectedError: nil,
		},
		{
			name: "list basic test",
			arg: cty.ObjectVal(map[string]cty.Value{
				"test": cty.TupleVal([]cty.Value{
					cty.StringVal("i1"),
					cty.StringVal("i2"),
					cty.NumberIntVal(3),
				}),
			}),
			want: cty.StringVal(`
				test = ["i1", "i2", 3]
			`),
		},
		{
			name: "list of objects",
			arg: cty.ObjectVal(map[string]cty.Value{
				"test": cty.TupleVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"o1k1": cty.StringVal("o1v1"),
					}),
					cty.ObjectVal(map[string]cty.Value{
						"o2k1": cty.StringVal("o2v1"),
						"o2k2": cty.ObjectVal(map[string]cty.Value{
							"o3k1": cty.StringVal("o3v1"),
						}),
					}),
				}),
			}),
			want: cty.StringVal(`
				test = [
					{
						o1k1 = "o1v1"
					},
					{
						o2k1 = "o2v1"
						o2k2 = {
							o3k1 = "o3v1"
						}
					}
				]
			`),
		},
		{
			name:          "null input",
			arg:           cty.NullVal(cty.DynamicPseudoType),
			want:          cty.StringVal(""),
			expectedError: errInvalidInput,
		},
		{
			name:          "invalid input: not an object",
			arg:           cty.StringVal("test"), // not an object
			want:          cty.StringVal(""),
			expectedError: errInvalidInput,
		},
		{
			name:          "invalid input: Object with invalid key",
			arg:           cty.ObjectVal(map[string]cty.Value{"7*7": cty.StringVal("test")}), // invalid key
			want:          cty.StringVal(""),
			expectedError: errInvalidInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encodeTFVars := &encodeTFVarsFunc{}
			got, err := encodeTFVars.Call([]cty.Value{tt.arg})
			if err != nil {
				if tt.expectedError == nil {
					t.Fatalf("Call() unexpected error: %v", err)
				}
				if !errors.Is(err, tt.expectedError) {
					t.Fatalf("Call() error = %v, expected %v", err, tt.expectedError)
				}
				return
			}

			formattedRequirement := formatHCLWithoutWhitespaces(tt.want)
			formattedGot := formatHCLWithoutWhitespaces(got)

			if formattedGot != formattedRequirement {
				t.Errorf("Call() got: %v, want: %v", formattedGot, formattedRequirement)
			}
		})
	}
}

func TestEncodeExprFunc(t *testing.T) {
	tests := []test{
		{
			name:          "string",
			arg:           cty.StringVal("test"),
			want:          cty.StringVal(`"test"`),
			expectedError: nil,
		},
		{
			name:          "number",
			arg:           cty.NumberIntVal(2),
			want:          cty.StringVal("2"),
			expectedError: nil,
		},
		{
			name:          "bool",
			arg:           cty.True,
			want:          cty.StringVal("true"),
			expectedError: nil,
		},
		{
			name: "null",
			arg:  cty.NullVal(cty.String),
			want: cty.StringVal("null"),
		},
		{
			name:          "tuple",
			arg:           cty.TupleVal([]cty.Value{cty.StringVal("test"), cty.StringVal("test2")}),
			want:          cty.StringVal(`["test", "test2"]`),
			expectedError: nil,
		},
		{
			name:          "tuple with objects",
			arg:           cty.TupleVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{"test": cty.StringVal("test")}), cty.ObjectVal(map[string]cty.Value{"test2": cty.StringVal("test2")})}),
			want:          cty.StringVal(`[{test = "test"}, {test2 = "test2"}]`),
			expectedError: nil,
		},
		{
			name:          "object",
			arg:           cty.ObjectVal(map[string]cty.Value{"test": cty.StringVal("test")}),
			want:          cty.StringVal(`{test = "test"}`),
			expectedError: nil,
		},
		{
			name:          "nested object",
			arg:           cty.ObjectVal(map[string]cty.Value{"test": cty.ObjectVal(map[string]cty.Value{"test": cty.StringVal("test")})}),
			want:          cty.StringVal(`{test = {test = "test"}}`),
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encodeExpr := &encodeExprFunc{}
			got, err := encodeExpr.Call([]cty.Value{tt.arg})
			if !errors.Is(err, tt.expectedError) {
				t.Errorf("Call() error = %v, expected %v", err, tt.expectedError)
			}
			formattedRequirement := formatHCLWithoutWhitespaces(tt.want)
			formattedGot := formatHCLWithoutWhitespaces(got)

			if formattedGot != formattedRequirement {
				t.Errorf("Call() got: %v, want: %v", formattedGot, formattedRequirement)
			}
		})
	}
}
