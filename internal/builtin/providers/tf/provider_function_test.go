package tf_test

import (
	"errors"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/builtin/providers/tf"
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
	f := string(hclwrite.Format([]byte(val.AsString())))
	f = strings.ReplaceAll(f, " ", "")
	f = strings.ReplaceAll(f, "\n", "")
	f = strings.ReplaceAll(f, "\t", "")
	return f
}

func TestEncodeTFVarsFunc(t *testing.T) {
	tests := []test{
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encodeTFVars := &tf.EncodeTFVarsFunc{}
			got, err := encodeTFVars.Call([]cty.Value{tt.arg})
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decodeTFVars := &tf.DecodeTFVarsFunc{}
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
			encodeExpr := &tf.EncodeExprFunc{}
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
