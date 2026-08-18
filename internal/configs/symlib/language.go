// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package symlib

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
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

	moreDiags = gohcl.DecodeExpression(content.Attributes["edition"].Expr, nil, &lang.Edition)
	diags = diags.Extend(moreDiags)

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
