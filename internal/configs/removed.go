// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/opentofu/opentofu/internal/addrs"
)

// Removed represents a removed block in the configuration.
type Removed struct {
	From *addrs.RemoveEndpoint

	Destroy    bool
	DestroySet bool

	DeclRange hcl.Range
}

func decodeRemovedBlock(block *hcl.Block) (*Removed, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	removed := &Removed{
		DeclRange: block.DefRange,
		Destroy:   false, // NOTE: false for backwards compatibility. This is not the same behavior that the other system is having.
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

	var seenLifecycle *hcl.Block
	for _, block := range content.Blocks {
		switch block.Type {
		case "lifecycle":
			if seenLifecycle != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate lifecycle block",
					Detail:   fmt.Sprintf("The removed block already has a lifecycle block at %s.", seenLifecycle.DefRange),
					Subject:  &block.DefRange,
				})
				continue
			}
			seenLifecycle = block

			lcContent, lcDiags := block.Body.Content(removedLifecycleBlockSchema)
			diags = append(diags, lcDiags...)

			if attr, exists := lcContent.Attributes["destroy"]; exists {
				valDiags := gohcl.DecodeExpression(attr.Expr, nil, &removed.Destroy)
				diags = append(diags, valDiags...)
				removed.DestroySet = true
			}
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
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "lifecycle"},
	},
}

var removedLifecycleBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name: "destroy",
		},
	},
}
