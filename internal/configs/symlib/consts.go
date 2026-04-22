package symlib

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

type Const struct {
	Name      string
	Expr      hcl.Expression
	DeclRange hcl.Range
}

func decodeConstBlock(block *hcl.Block) ([]*Const, hcl.Diagnostics) {
	var consts []*Const
	attrs, diags := block.Body.JustAttributes()
	for name, attr := range attrs {
		if !hclsyntax.ValidIdentifier(name) {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid const value name",
				Detail:   badIdentifierDetail,
				Subject:  &attr.NameRange,
			})
		}
		consts = append(consts, &Const{
			Name:      name,
			Expr:      attr.Expr,
			DeclRange: attr.Range,
		})
	}
	return consts, diags
}
