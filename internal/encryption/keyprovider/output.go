// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package keyprovider

import "github.com/zclconf/go-cty/cty"

// Output is the standardized structure a key provider must return when providing a key.
// It contains two keys because some key providers may prefer include random data (e.g. salt)
// in the generated keys and this salt will be different for decryption and encryption.
type Output struct {
	EncryptionKey []byte `hcl:"encryption_key" cty:"encryption_key" json:"encryption_key,omitempty" yaml:"encryption_key"`
	DecryptionKey []byte `hcl:"decryption_key,optional" cty:"decryption_key" json:"decryption_key,omitempty" yaml:"decryption_key"`
}

// Cty turns the Output struct into a CTY value.
func (o *Output) Cty() cty.Value {
	return cty.ObjectVal(map[string]cty.Value{
		"encryption_key": o.byteToCty(o.EncryptionKey),
		"decryption_key": o.byteToCty(o.DecryptionKey),
	})
}

func (o *Output) byteToCty(data []byte) cty.Value {
	if len(data) == 0 {
		return cty.NullVal(cty.List(cty.Number))
	}
	ctyData := make([]cty.Value, len(data))
	for i, d := range data {
		ctyData[i] = cty.NumberIntVal(int64(d))
	}
	return cty.ListVal(ctyData)
}
