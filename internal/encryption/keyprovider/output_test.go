// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package keyprovider_test

import (
	"testing"

	"github.com/terramate-io/opentofulib/internal/encryption/keyprovider"
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
