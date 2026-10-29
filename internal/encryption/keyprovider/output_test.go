// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package keyprovider_test

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/zclconf/go-cty/cty"
)

func TestOutputCty(t *testing.T) {
	testCases := map[string]struct {
		output         keyprovider.Output
		expectedOutput cty.Value
	}{
		"empty": {
			output: keyprovider.Output{},
			expectedOutput: cty.ObjectVal(map[string]cty.Value{
				"encryption_key": cty.NullVal(cty.List(cty.Number)),
				"decryption_key": cty.NullVal(cty.List(cty.Number)),
			}),
		},
		"encryption-key-only": {
			output: keyprovider.Output{
				EncryptionKey: []byte("Hello world!"),
			},
			expectedOutput: cty.ObjectVal(map[string]cty.Value{
				"encryption_key": cty.ListVal([]cty.Value{
					cty.NumberIntVal(int64('H')),
					cty.NumberIntVal(int64('e')),
					cty.NumberIntVal(int64('l')),
					cty.NumberIntVal(int64('l')),
					cty.NumberIntVal(int64('o')),
					cty.NumberIntVal(int64(' ')),
					cty.NumberIntVal(int64('w')),
					cty.NumberIntVal(int64('o')),
					cty.NumberIntVal(int64('r')),
					cty.NumberIntVal(int64('l')),
					cty.NumberIntVal(int64('d')),
					cty.NumberIntVal(int64('!')),
				}),
				"decryption_key": cty.NullVal(cty.List(cty.Number)),
			}),
		},
		"both-keys": {
			output: keyprovider.Output{
				EncryptionKey: []byte("Hello world!"),
				DecryptionKey: []byte("Hello world!"),
			},
			expectedOutput: cty.ObjectVal(map[string]cty.Value{
				"encryption_key": cty.ListVal([]cty.Value{
					cty.NumberIntVal(int64('H')),
					cty.NumberIntVal(int64('e')),
					cty.NumberIntVal(int64('l')),
					cty.NumberIntVal(int64('l')),
					cty.NumberIntVal(int64('o')),
					cty.NumberIntVal(int64(' ')),
					cty.NumberIntVal(int64('w')),
					cty.NumberIntVal(int64('o')),
					cty.NumberIntVal(int64('r')),
					cty.NumberIntVal(int64('l')),
					cty.NumberIntVal(int64('d')),
					cty.NumberIntVal(int64('!')),
				}),
				"decryption_key": cty.ListVal([]cty.Value{
					cty.NumberIntVal(int64('H')),
					cty.NumberIntVal(int64('e')),
					cty.NumberIntVal(int64('l')),
					cty.NumberIntVal(int64('l')),
					cty.NumberIntVal(int64('o')),
					cty.NumberIntVal(int64(' ')),
					cty.NumberIntVal(int64('w')),
					cty.NumberIntVal(int64('o')),
					cty.NumberIntVal(int64('r')),
					cty.NumberIntVal(int64('l')),
					cty.NumberIntVal(int64('d')),
					cty.NumberIntVal(int64('!')),
				}),
			}),
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			val := tc.output.Cty()
			if !val.Equals(tc.expectedOutput).True() {
				t.Fatalf("Incorrect cty output value:\n%v\nexpected:\n%v)", val, tc.expectedOutput)
			}
		})
	}
}

func TestDecodeOutput(t *testing.T) {
	src := hcl.Range{
		Filename: "test.tf",
		Start:    hcl.Pos{Line: 1, Column: 1, Byte: 1},
		End:      hcl.Pos{Line: 1, Column: 1, Byte: 1},
	}
	testCases := map[string]struct {
		input          cty.Value
		expectedOutput keyprovider.Output
		expectedDiags  hcl.Diagnostics
	}{
		"wrong type - string": {
			input:         cty.StringVal("wrong value"),
			expectedDiags: hcl.Diagnostics{&hcl.Diagnostic{Summary: "Expected key_provider value", Detail: "Expected a key_provider compatible value, found string instead", Subject: &src}},
		},
		"wrong type - number": {
			input:         cty.NumberFloatVal(4.2),
			expectedDiags: hcl.Diagnostics{&hcl.Diagnostic{Summary: "Expected key_provider value", Detail: "Expected a key_provider compatible value, found number instead", Subject: &src}},
		},
		"wrong type - list": {
			input:         cty.ListVal([]cty.Value{cty.StringVal("val")}),
			expectedDiags: hcl.Diagnostics{&hcl.Diagnostic{Summary: "Expected key_provider value", Detail: "Expected a key_provider compatible value, found list of string instead", Subject: &src}},
		},
		"wrong type - set": {
			input:         cty.SetVal([]cty.Value{cty.StringVal("val")}),
			expectedDiags: hcl.Diagnostics{&hcl.Diagnostic{Summary: "Expected key_provider value", Detail: "Expected a key_provider compatible value, found set of string instead", Subject: &src}},
		},
		"wrong map content": {
			input:         cty.MapVal(map[string]cty.Value{"wrong_key": cty.StringVal("just a value")}),
			expectedDiags: hcl.Diagnostics{&hcl.Diagnostic{Summary: "Missing encryption_key value", Detail: "An encryption_key value is required in the key_provider compatible object at this location", Subject: &src}},
		},
		"wrong object content": {
			input:         cty.ObjectVal(map[string]cty.Value{"wrong_key": cty.StringVal("just a value")}),
			expectedDiags: hcl.Diagnostics{&hcl.Diagnostic{Summary: "Missing encryption_key value", Detail: "An encryption_key value is required in the key_provider compatible object at this location", Subject: &src}},
		},
		"map with a wrong type of the encryption_key entry": {
			input:         cty.MapVal(map[string]cty.Value{"encryption_key": cty.StringVal("just a value")}),
			expectedDiags: hcl.Diagnostics{&hcl.Diagnostic{Summary: "Unsuitable value type", Detail: "Unsuitable value: list of number required, but have string", Subject: &src}},
		},
		"object with a wrong type of the encryption_key entry": {
			input:         cty.ObjectVal(map[string]cty.Value{"encryption_key": cty.StringVal("just a value")}),
			expectedDiags: hcl.Diagnostics{&hcl.Diagnostic{Summary: "Unsuitable value type", Detail: "Unsuitable value: list of number required, but have string", Subject: &src}},
		},
		"map with valid encryption_key": {
			input:          cty.MapVal(map[string]cty.Value{"encryption_key": cty.ListVal([]cty.Value{cty.NumberIntVal(int64('k')), cty.NumberIntVal(int64('e')), cty.NumberIntVal(int64('y'))})}),
			expectedOutput: keyprovider.Output{EncryptionKey: []byte("key")},
		},
		"object with valid encryption_key": {
			input:          cty.ObjectVal(map[string]cty.Value{"encryption_key": cty.ListVal([]cty.Value{cty.NumberIntVal(int64('k')), cty.NumberIntVal(int64('e')), cty.NumberIntVal(int64('y'))})}),
			expectedOutput: keyprovider.Output{EncryptionKey: []byte("key")},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			output, diags := keyprovider.DecodeOutput(tc.input, src)
			gotErr := diags.Error()
			wantErr := tc.expectedDiags.Error()
			if gotErr != wantErr {
				t.Errorf("unexpected returned error\n\tgot: %s\n\twant: %s", gotErr, wantErr)
			}
			gotVal := output.Cty()
			wantVal := tc.expectedOutput.Cty()
			if !gotVal.Equals(wantVal).True() {
				t.Fatalf("incorrect output value\n\tgot: %s\n\twant: %s", gotVal.GoString(), wantVal.GoString())
			}
		})
	}
}
