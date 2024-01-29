// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
)

type Removed struct {
	From *addrs.RemoveEndpoint

	DeclRange hcl.Range
}

func decodeRemovedBlock(block *hcl.Block) (*Removed, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	removed := &Removed{
		DeclRange: block.DefRange,
	}

	content, moreDiags := block.Body.Content(removedBlockSchema)
	diags = append(diags, moreDiags...)

	if attr, exists := content.Attributes["from"]; exists {
		from, traversalDiags := hcl.AbsTraversalForExpr(attr.Expr)
		diags = append(diags, traversalDiags...)
		if !traversalDiags.HasErrors() {
			from, fromDiags := addrs.ParseRemoveEndpoint(from)
			diags = append(diags, fromDiags.ToHCL()...)
			removed.From = from
		}
	}

	return removed, diags
}

var removedBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name:     "from",
			Required: true,
		},
	},
}
