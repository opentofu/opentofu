package symlib

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

type TypeDef struct {
	Name      string
	TypeExpr  hcl.Expression
	DeclRange hcl.Range
}

func decodeTypeDefBlock(block *hcl.Block) (*TypeDef, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	td := &TypeDef{
		Name:      block.Labels[0],
		DeclRange: block.DefRange,
	}

	content, moreDiags := block.Body.Content(typeDefBlockSchema)
	diags = diags.Extend(moreDiags)

	if !hclsyntax.ValidIdentifier(td.Name) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid typedef name",
			Detail:   badIdentifierDetail,
			Subject:  &block.LabelRanges[0],
		})
	}

	td.TypeExpr = content.Attributes["type"].Expr

	return td, diags
}

var typeDefBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name:     "type",
			Required: true,
		},
	},
}
