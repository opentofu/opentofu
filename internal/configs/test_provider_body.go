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

func (c testProviderBody) Content(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Diagnostics) {
	content, diags := c.originalBody.Content(schema)
	if diags.HasErrors() {
		return nil, diags
	}
	attrs := content.Attributes
	for name, attr := range attrs {
		vars := attr.Expr.Variables()
		containsRunRef := false
		for _, v := range vars {
			if v.RootName() == "run" {
				containsRunRef = true
				break
			}
		}
		if !containsRunRef {
			continue
		}

		attr, valueDiags := c.getLiteralAttr(attr)
		diags = append(diags, valueDiags...)
		if diags.HasErrors() {
			continue
		}
		attrs[name] = attr
	}
	return content, diags
}

func (c testProviderBody) PartialContent(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Body, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	partialContent, remain, diags := c.originalBody.PartialContent(schema)

	if diags.HasErrors() {
		return nil, nil, diags
	}

	for name, attr := range partialContent.Attributes {
		vars := attr.Expr.Variables()
		containsRunRef := false
		for _, v := range vars {
			if v.RootName() == "run" {
				containsRunRef = true
				break
			}
		}
		if !containsRunRef {
			continue
		}
		attr, valueDiags := c.getLiteralAttr(attr)
		diags = append(diags, valueDiags...)
		if diags.HasErrors() {
			continue
		}
		partialContent.Attributes[name] = attr
	}

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
