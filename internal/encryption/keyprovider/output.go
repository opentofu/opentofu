// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package keyprovider

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// Output is the standardized structure a key provider must return when providing a key.
// It contains two keys because some key providers may prefer include random data (e.g. salt)
// in the generated keys and this salt will be different for decryption and encryption.
type Output struct {
	EncryptionKey []byte `hcl:"encryption_key" cty:"encryption_key" json:"encryption_key,omitempty" yaml:"encryption_key"`
	DecryptionKey []byte `hcl:"decryption_key,optional" cty:"decryption_key" json:"decryption_key,omitempty" yaml:"decryption_key"`
}

func DecodeOutput(val cty.Value, subject hcl.Range) (Output, hcl.Diagnostics) {
	var out Output
	if !val.CanIterateElements() {
		return out, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Expected key_provider value",
			Detail:   fmt.Sprintf("Expected a key_provider compatible value, found %s instead", val.Type().FriendlyName()),
			Subject:  &subject,
		}}
	}

	var diags hcl.Diagnostics
	mapVal := val.AsValueMap()
	if attr, ok := mapVal["encryption_key"]; ok {
		decodeDiags := gohcl.DecodeExpression(&hclsyntax.LiteralValueExpr{Val: attr, SrcRange: subject}, nil, &out.EncryptionKey)
		diags = diags.Extend(decodeDiags)
	} else {
		return out, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Missing encryption_key value",
			Detail:   "An encryption_key value is required in the key_provider compatible object at this location",
			Subject:  &subject,
		}}
	}

	if attr, ok := mapVal["decryption_key"]; ok {
		decodeDiags := gohcl.DecodeExpression(&hclsyntax.LiteralValueExpr{Val: attr, SrcRange: subject}, nil, &out.DecryptionKey)
		diags = diags.Extend(decodeDiags)
	}
	return out, diags
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
