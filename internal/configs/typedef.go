package configs

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

func (l *Library) TypeContext() (*typeexpr.TypeContext, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	namespace := "types" // todo modules

	typeCtx := &typeexpr.TypeContext{
		Types:    map[string]map[string]cty.Type{namespace: {}},
		Defaults: map[string]map[string]*typeexpr.Defaults{namespace: {}},
	}

	// For now, we load in a random order.  Long term this is a complex dependency step
	for _, typeDef := range l.TypeDefs {
		varType, typeDefault, valDiags := typeCtx.TypeConstraintWithDefaults(typeDef.TypeExpr)
		diags = diags.Extend(valDiags)

		typeCtx.Types[namespace][typeDef.Name] = varType
		if typeDefault != nil {
			typeCtx.Defaults[namespace][typeDef.Name] = typeDefault
		}

	}

	return typeCtx, diags
}

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
