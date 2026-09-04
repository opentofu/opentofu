// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package symlib

import (
	"github.com/hashicorp/hcl/v2"
)

type Language struct {
	Edition   string
	DeclRange hcl.Range
}

func decodeLanguageBlock(block *hcl.Block) (*Language, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	lang := &Language{
		DeclRange: block.DefRange,
	}

	content, moreDiags := block.Body.Content(languageBlockSchema)
	diags = diags.Extend(moreDiags)

	if attr, ok := content.Attributes["edition"]; ok {
		lang.Edition = hcl.ExprAsKeyword(attr.Expr)
		if lang.Edition == "" { // (the expression wasn't a keyword at all)
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid language edition",
				Detail:   "The \"edition\" argument expects a bare language edition keyword.",
				Subject:  attr.Expr.Range().Ptr(),
			})
		}
	}

	return lang, diags
}

var languageBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name:     "edition",
			Required: true,
		},
	},
}
