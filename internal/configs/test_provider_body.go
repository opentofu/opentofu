// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

var _ hcl.Body = &testProviderBody{}

// testProviderBody is a wrapper around hcl.Body that allows us to use prepared EvalContext for provider configuration.
type testProviderBody struct {
	originalBody hcl.Body
	evalCtx      *hcl.EvalContext
}

func (c testProviderBody) evaluateBodyContent(content *hcl.BodyContent) (*hcl.BodyContent, hcl.Diagnostics) {
	attrs := content.Attributes
	var diags hcl.Diagnostics
	for name, attr := range attrs {
		attr, valueDiags := c.getLiteralAttr(attr)
		diags = append(diags, valueDiags...)
		if diags.HasErrors() {
			continue
		}
		attrs[name] = attr
	}

	// Recursively evaluate nested blocks
	for name, block := range content.Blocks {
		block.Body = testProviderBody{originalBody: block.Body, evalCtx: c.evalCtx}
		content.Blocks[name] = block
	}
	return content, diags
}

func (c testProviderBody) Content(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Diagnostics) {
	content, diags := c.originalBody.Content(schema)
	if diags.HasErrors() {
		return nil, diags
	}
	return c.evaluateBodyContent(content)
}

func (c testProviderBody) PartialContent(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Body, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	partialContent, remain, diags := c.originalBody.PartialContent(schema)

	if diags.HasErrors() {
		return nil, nil, diags
	}

	partialContent, diags = c.evaluateBodyContent(partialContent)

	return partialContent, testProviderBody{originalBody: remain, evalCtx: c.evalCtx}, diags

}

func (c testProviderBody) JustAttributes() (hcl.Attributes, hcl.Diagnostics) {
	attrs, diags := c.originalBody.JustAttributes()
	for name, attr := range attrs {
		attr, valueDiags := c.getLiteralAttr(attr)
		diags = append(diags, valueDiags...)
		if diags.HasErrors() {
			continue
		}
		attrs[name] = attr
	}
	return attrs, diags
}

func (c testProviderBody) MissingItemRange() hcl.Range {
	return c.originalBody.MissingItemRange()
}

func (c testProviderBody) getLiteralAttr(attr *hcl.Attribute) (*hcl.Attribute, hcl.Diagnostics) {
	val, diags := attr.Expr.Value(c.evalCtx)
	if diags.HasErrors() {
		return nil, diags
	}
	return &hcl.Attribute{
		Name: attr.Name,
		Expr: &hclsyntax.LiteralValueExpr{
			Val:      val,
			SrcRange: attr.Range,
		},
		Range:     attr.Range,
		NameRange: attr.NameRange,
	}, nil
}
