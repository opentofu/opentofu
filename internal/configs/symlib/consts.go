package symlib

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

type Value struct {
	Name      string
	Expr      hcl.Expression
	DeclRange hcl.Range
}

func decodeValuesBlock(block *hcl.Block) ([]*Value, hcl.Diagnostics) {
	var values []*Value
	attrs, diags := block.Body.JustAttributes()
	for name, attr := range attrs {
		if !hclsyntax.ValidIdentifier(name) {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid value name",
				Detail:   badIdentifierDetail,
				Subject:  &attr.NameRange,
			})
		}
		values = append(values, &Value{
			Name:      name,
			Expr:      attr.Expr,
			DeclRange: attr.Range,
		})
	}
	return values, diags
}
