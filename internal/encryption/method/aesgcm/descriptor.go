// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aesgcm

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/method"
)

// New creates a new descriptor for the AES-GCM encryption method, which requires a 32-byte key.
func New() method.Descriptor {
	return &descriptor{}
}

type descriptor struct {
}

func (f *descriptor) ID() method.ID {
	return "aes_gcm"
}

func (f *descriptor) DecodeConfig(methodCtx method.EvalContext, body hcl.Body) (method.Config, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	methodCfg := &Config{}

	content, contentDiags := body.Content(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "keys", Required: true},
			{Name: "aad", Required: false},
		},
	})
	diags = diags.Extend(contentDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	keyExpr := content.Attributes["keys"].Expr
	// keyExpr can either be raw data/references to raw data or a string reference to a key provider (JSON support)
	keyVal, keyDiags := methodCtx.ValueForExpression(keyExpr)
	diags = diags.Extend(keyDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	methodCfg.Keys, keyDiags = keyprovider.DecodeOutput(keyVal, keyExpr.Range())
	diags = diags.Extend(keyDiags)

	if attr, ok := content.Attributes["aad"]; ok {
		attrVal, attrDiags := methodCtx.ValueForExpression(attr.Expr)
		diags = diags.Extend(attrDiags)

		decodeDiags := gohcl.DecodeExpression(&hclsyntax.LiteralValueExpr{Val: attrVal, SrcRange: attr.Expr.Range()}, nil, &methodCfg.AAD)
		diags = diags.Extend(decodeDiags)
	}

	return methodCfg, diags
}
