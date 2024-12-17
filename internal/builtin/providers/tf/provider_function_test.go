package tf_test

import (
	"errors"
	"testing"

	"github.com/opentofu/opentofu/internal/builtin/providers/tf"
	"github.com/zclconf/go-cty/cty"
)

type test struct {
	name          string
	args          []cty.Value
	want          cty.Value
	expectedError error
}

func TestEncodeTFVarsFunc(t *testing.T) {
	tests := []test{
		{
			name: "encode_tfvars basic test",
			args: []cty.Value{
				cty.StringVal("test"),
			},
			want:          cty.StringVal("test"),
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encodeTFVars := &tf.EncodeTFVarsFunc{}
			got, err := encodeTFVars.Call(tt.args)
			if !errors.Is(err, tt.expectedError) {
				t.Errorf("Call() error = %v, expected %v", err, tt.expectedError)
			}

			if got.NotEqual(tt.want).True() {
				t.Errorf("Call() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecodeTFVarsFunc(t *testing.T) {
	tests := []test{
		{
			name: "decode_tfvars basic test",
			args: []cty.Value{
				cty.StringVal(`
					test = 2
				`),
			},
			want: cty.ObjectVal(map[string]cty.Value{
				"test": cty.NumberIntVal(2),
			}),
			expectedError: nil,
		},
		{
			name: "decode_tfvars object basic test",
			args: []cty.Value{
				cty.StringVal(`
					test = {
						k = "v"
					}
				`),
			},
			want: cty.ObjectVal(map[string]cty.Value{
				"test": cty.ObjectVal(map[string]cty.Value{
					"k": cty.StringVal("v"),
				}),
			}),
			expectedError: nil,
		},
		{
			name: "decode_tfvars list basic test",
			args: []cty.Value{
				cty.StringVal(`
					test = [
						"i1",
						"i2",
						3
					]
				`),
			},
			want: cty.ObjectVal(map[string]cty.Value{
				"test": cty.TupleVal([]cty.Value{
					cty.StringVal("i1"),
					cty.StringVal("i2"),
					cty.NumberIntVal(3),
				}),
			}),
		},
		{
			name: "decode_tfvars list of objects",
			args: []cty.Value{
				cty.StringVal(`
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
			got, err := decodeTFVars.Call(tt.args)
			if !errors.Is(err, tt.expectedError) {
				t.Errorf("Call() error = %v, expected %v", err, tt.expectedError)
			}
			if got.NotEqual(tt.want).True() {
				t.Errorf("Call() got = %v, want %v", got, tt.want)
			}
		})
	}
}
