// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package external

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

func (d *descriptor) ID() method.ID {
	return "external"
}

func (d *descriptor) DecodeConfig(methodCtx method.EvalContext, body hcl.Body) (method.Config, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	methodCfg := &Config{}

	content, contentDiags := body.Content(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "keys", Required: false},
			{Name: "encrypt_command", Required: true},
			{Name: "decrypt_command", Required: true},
		},
	})
	diags = diags.Extend(contentDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	if keyAttr, ok := content.Attributes["keys"]; ok {
		keyExpr := keyAttr.Expr
		// keyExpr can either be raw data/references to raw data or a string reference to a key provider (JSON support)
		keyVal, keyDiags := methodCtx.ValueForExpression(keyExpr)
		diags = diags.Extend(keyDiags)
		if diags.HasErrors() {
			return nil, diags
		}
		keys, decodeDiags := keyprovider.DecodeOutput(keyVal, keyExpr.Range())
		diags = diags.Extend(decodeDiags)
		if diags.HasErrors() {
			return nil, diags
		}
		methodCfg.Keys = &keys
	}

	encryptAttr := content.Attributes["encrypt_command"]
	encryptVal, valueDiags := methodCtx.ValueForExpression(encryptAttr.Expr)
	diags = diags.Extend(valueDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	decodeEncryptCmdDiags := gohcl.DecodeExpression(&hclsyntax.LiteralValueExpr{Val: encryptVal, SrcRange: encryptAttr.Expr.Range()}, nil, &methodCfg.EncryptCommand)
	diags = diags.Extend(decodeEncryptCmdDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	decryptAttr := content.Attributes["decrypt_command"]
	decryptVal, valueDiags := methodCtx.ValueForExpression(decryptAttr.Expr)
	diags = diags.Extend(valueDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	decodeDecryptCmdDiags := gohcl.DecodeExpression(&hclsyntax.LiteralValueExpr{Val: decryptVal, SrcRange: decryptAttr.Expr.Range()}, nil, &methodCfg.DecryptCommand)
	diags = diags.Extend(decodeDecryptCmdDiags)

	return methodCfg, diags
}
