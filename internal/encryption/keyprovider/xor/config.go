// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package xor

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
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

func (c *Config) DepsTraversals(body hcl.Body) ([]hcl.Traversal, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	aTrav, _, aDiags := extractKeyProviderAttrTraversal(body, "a")
	diags = diags.Extend(aDiags)
	bTrav, _, bDiags := extractKeyProviderAttrTraversal(body, "b")
	diags = diags.Extend(bDiags)
	if diags.HasErrors() {
		return nil, diags
	}
	var ret []hcl.Traversal
	if aTrav != nil {
		ret = append(ret, aTrav)
	}
	if bTrav != nil {
		ret = append(ret, bTrav)
	}
	return ret, nil
}

func (c *Config) DecodeConfig(body hcl.Body, evalCtx *hcl.EvalContext) (diags hcl.Diagnostics) {
	aTrav, aAttr, aDiags := extractKeyProviderAttrTraversal(body, "a")
	diags = diags.Extend(aDiags)
	bTrav, bAttr, bDiags := extractKeyProviderAttrTraversal(body, "b")
	diags = diags.Extend(bDiags)
	if diags.HasErrors() {
		return diags
	}
	if aTrav != nil {
		val, exprDiags := aTrav.TraverseAbs(evalCtx)
		diags = diags.Extend(exprDiags)
		if exprDiags.HasErrors() {
			return diags
		}
		// NOTE: at this point, val cannot be [cty.NilVal] or null since this will be caught after traversal
		// returned from [Config.DepsTraversals].
		out, outDiags := keyprovider.DecodeOutput(val, aAttr.Expr.Range())
		diags = diags.Extend(outDiags)
		if outDiags.HasErrors() {
			return diags
		}
		c.A = out
	}
	if bTrav != nil {
		val, exprDiags := bTrav.TraverseAbs(evalCtx)
		diags = diags.Extend(exprDiags)
		if exprDiags.HasErrors() {
			return diags
		}
		// NOTE: at this point, val cannot be [cty.NilVal] or null since this will be caught after traversal
		// returned from [Config.DepsTraversals].
		out, outDiags := keyprovider.DecodeOutput(val, bAttr.Expr.Range())
		diags = diags.Extend(outDiags)
		if outDiags.HasErrors() {
			return diags
		}
		c.B = out
	}
	// Nothing else to decode here
	return diags
}

func extractKeyProviderAttrTraversal(body hcl.Body, attrKey string) (hcl.Traversal, *hcl.Attribute, hcl.Diagnostics) {
	if body == nil {
		return nil, nil, nil
	}
	var diags hcl.Diagnostics
	content, _, contentDiags := body.PartialContent(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			// For the dependencies purpose, we want to get only the chain expression out of the body
			{Name: attrKey, Required: false},
		},
	})
	diags = diags.Extend(contentDiags)
	if diags.HasErrors() {
		return nil, nil, diags
	}
	attr, ok := content.Attributes[attrKey]
	if !ok {
		return nil, nil, diags
	}
	traversals := attr.Expr.Variables()
	if len(traversals) == 1 {
		return traversals[0], attr, nil
	}
	traversal, exprDiags := hcl.AbsTraversalForExpr(attr.Expr)
	return traversal, attr, diags.Extend(exprDiags)
}
