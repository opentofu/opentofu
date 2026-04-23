package symlib

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

type LibraryCall struct {
	Name        string
	Source      hcl.Expression
	VersionAttr *hcl.Attribute
	DeclRange   hcl.Range
}

func DecodeLibraryBlock(block *hcl.Block) (*LibraryCall, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	lc := &LibraryCall{
		DeclRange: block.DefRange,
		Name:      block.Labels[0],
	}

	content, moreDiags := block.Body.Content(libraryBlockSchema)
	diags = append(diags, moreDiags...)

	if !hclsyntax.ValidIdentifier(lc.Name) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid library name",
			Detail:   badIdentifierDetail,
			Subject:  &block.LabelRanges[0],
		})
	}

	if attr, exists := content.Attributes["version"]; exists {
		lc.VersionAttr = attr
	}

	if attr, exists := content.Attributes["source"]; exists {
		lc.Source = attr.Expr
	}

	return lc, diags
}

var libraryBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name:     "source",
			Required: true,
		},
		{
			Name: "version",
		},
	},
}
