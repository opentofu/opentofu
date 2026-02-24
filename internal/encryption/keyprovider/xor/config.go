// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package xor

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/zclconf/go-cty/cty"
)

// Config contains the configuration for this key provider supplied by the user. This struct must have hcl tags in order
// to function.
type Config struct {
	A keyprovider.Output `hcl:"a"`
	B keyprovider.Output `hcl:"b"`
}

// Build will create the usable key provider.
func (c *Config) Build() (keyprovider.KeyProvider, keyprovider.KeyMeta, error) {
	if len(c.A.EncryptionKey) == 0 {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: "Missing A encryption key",
		}
	}
	if len(c.B.EncryptionKey) == 0 {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: "Missing B encryption key",
		}
	}
	if len(c.A.EncryptionKey) != len(c.B.EncryptionKey) {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: fmt.Sprintf("The two provided encryption keys are not equal in length (%d vs %d bytes)", len(c.A.EncryptionKey), len(c.B.EncryptionKey)),
		}
	}
	if len(c.A.DecryptionKey) != len(c.B.DecryptionKey) {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: fmt.Sprintf("The two provided decryption keys are not equal in length (%d vs %d bytes)", len(c.A.DecryptionKey), len(c.B.DecryptionKey)),
		}
	}

	encryptionKey := make([]byte, len(c.A.EncryptionKey))
	for i := range c.A.EncryptionKey {
		encryptionKey[i] = c.A.EncryptionKey[i] ^ c.B.EncryptionKey[i]
	}
	decryptionKey := make([]byte, len(c.A.DecryptionKey))
	for i := range c.A.DecryptionKey {
		decryptionKey[i] = c.A.DecryptionKey[i] ^ c.B.DecryptionKey[i]
	}
	return &xorKeyProvider{keyprovider.Output{
		EncryptionKey: encryptionKey,
		DecryptionKey: decryptionKey,
	}}, nil, nil
}

func (c *Config) DecodeConfig(body hcl.Body, evalCtx *hcl.EvalContext) (diags hcl.Diagnostics) {
	if body == nil {
		return diags
	}
	content, contentDiags := body.Content(c.ConfigSchema())
	diags = diags.Extend(contentDiags)
	if contentDiags.HasErrors() {
		return diags
	}
	if attr, ok := content.Attributes["a"]; ok {
		value, vDiags := evaluateExpr(attr.Expr, evalCtx)
		diags = diags.Extend(vDiags)
		if !vDiags.HasErrors() {
			out, outDiags := keyprovider.DecodeOutput(value, attr.Range)
			diags = diags.Extend(outDiags)
			c.A = out
		}
	}
	if attr, ok := content.Attributes["b"]; ok {
		value, vDiags := evaluateExpr(attr.Expr, evalCtx)
		diags = diags.Extend(vDiags)
		if !vDiags.HasErrors() {
			out, outDiags := keyprovider.DecodeOutput(value, attr.Range)
			diags = diags.Extend(outDiags)
			c.B = out
		}
	}
	// Nothing else to decode here
	return diags
}

func (c *Config) ConfigSchema() *hcl.BodySchema {
	return &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "a", Required: false},
			{Name: "b", Required: false},
		},
	}
}

// evaluateExpr tries to evaluate the expression in different ways.
//   - Evaluate by using the traversals returned by the Variables() call
//   - If the first step does not work, tries to convert the expression into an absolute traversal and use that new traversal
//     to generate a value.
func evaluateExpr(expr hcl.Expression, evalCtx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	traversals := expr.Variables()
	// We are interested only in situations where the `chain` attribute contains exactly one key provider reference
	if len(traversals) == 1 {
		return traversals[0].TraverseAbs(evalCtx)
	}

	traversal, exprDiags := hcl.AbsTraversalForExpr(expr)
	if exprDiags.HasErrors() || traversal == nil {
		return cty.NilVal, exprDiags
	}
	return traversal.TraverseAbs(evalCtx)
}
