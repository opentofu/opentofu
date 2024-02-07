// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Import struct {
	ID hcl.Expression

	To         hcl.Expression
	StaticTo   addrs.ConfigResource
	ResolvedTo *addrs.AbsResourceInstance

	ProviderConfigRef *ProviderConfigRef
	Provider          addrs.Provider

	DeclRange         hcl.Range
	ProviderDeclRange hcl.Range
}

func decodeImportBlock(block *hcl.Block) (*Import, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	imp := &Import{
		DeclRange: block.DefRange,
	}

	content, moreDiags := block.Body.Content(importBlockSchema)
	diags = append(diags, moreDiags...)

	if attr, exists := content.Attributes["id"]; exists {
		imp.ID = attr.Expr
	}

	if attr, exists := content.Attributes["to"]; exists {
		imp.To = attr.Expr
		staticAddress, addressDiags := StaticImportAddress(attr.Expr)
		diags = append(diags, addressDiags.ToHCL()...)

		// Exit early if there are issues resolving the address. We wouldn't be able to validate the provider in such a case
		if diags.HasErrors() {
			return imp, diags
		}
		imp.StaticTo = staticAddress

		imp.ResolvedTo = ResolvedImportAddress(imp.To)
	}

	if attr, exists := content.Attributes["provider"]; exists {
		if len(imp.StaticTo.Module) > 0 {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid import provider argument",
				Detail:   "The provider argument can only be specified in import blocks that will generate configuration.\n\nUse the providers argument within the module block to configure providers for all resources within a module, including imported resources.",
				Subject:  attr.Range.Ptr(),
			})
		}

		var providerDiags hcl.Diagnostics
		imp.ProviderConfigRef, providerDiags = decodeProviderConfigRef(attr.Expr, "provider")
		imp.ProviderDeclRange = attr.Range
		diags = append(diags, providerDiags...)
	}

	return imp, diags
}

var importBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name: "provider",
		},
		{
			Name:     "id",
			Required: true,
		},
		{
			Name:     "to",
			Required: true,
		},
	},
}

// AbsTraversalForImportToExpr returns a static traversal of an import block's "to" field.
// It is inspired by hcl.AbsTraversalForExpr and by tofu.triggersExprToTraversal
// The use-case here is different - we want to also allow for hclsyntax.IndexExpr to be allowed,
// but we don't really care about the key part of it. We just want a traversal that could be converted to an address
// of a resource, so we could determine the module + resource + provider
//
// Currently, there are 4 types of HCL epressions that support AsTraversal:
// - hclsyntax.ScopeTraversalExpr - Simply returns the Traversal. Same for our use-case here
// - hclsyntax.RelativeTraversalExpr - Calculates hcl.AbsTraversalForExpr for the Source, and adds the Traversal to it. Same here, with AbsTraversalForImportToExpr instead
// - hclsyntax.LiteralValueExpr - Mainly for null/false/true values. Not relevant in our use-case, as it's could not really be part of a reference (unless it is inside of an index, which is irrelevant here anyway)
// - hclsyntax.ObjectConsKeyExpr - Not relevant here
//
// In addition to these, we need to also support hclsyntax.IndexExpr. For it - we do not care about what's in the index.
// We need need only know that parts of it the "Collection" part of it, as the index doesn't affect which resource/module this is
func AbsTraversalForImportToExpr(expr hcl.Expression) (traversal hcl.Traversal, diags tfdiags.Diagnostics) {
	physExpr := hcl.UnwrapExpressionUntil(expr, func(expr hcl.Expression) bool {
		switch expr.(type) {
		case *hclsyntax.IndexExpr, *hclsyntax.ScopeTraversalExpr, *hclsyntax.RelativeTraversalExpr:
			return true
		default:
			return false
		}
	})

	switch e := physExpr.(type) {
	case *hclsyntax.IndexExpr:
		t, d := AbsTraversalForImportToExpr(e.Collection)
		diags = diags.Append(d)
		traversal = append(traversal, t...)
	case *hclsyntax.RelativeTraversalExpr:
		t, d := AbsTraversalForImportToExpr(e.Source)
		diags = diags.Append(d)
		traversal = append(traversal, t...)
		traversal = append(traversal, e.Traversal...)
	case *hclsyntax.ScopeTraversalExpr:
		traversal = append(traversal, e.Traversal...)
	default:
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid import address expression",
			Detail:   "A single static variable reference is required: only attribute access and indexing with constant keys. No calculations, function calls, template expressions, etc are allowed here.",
			Subject:  expr.Range().Ptr(),
		}) // TODO indexing with constant keys are supported? Check out how
	}
	return
}

func StaticImportAddress(expr hcl.Expression) (addrs.ConfigResource, tfdiags.Diagnostics) {
	traversal, diags := AbsTraversalForImportToExpr(expr)
	if diags.HasErrors() {
		return addrs.ConfigResource{}, diags
	}

	absResourceInstance, diags := addrs.ParseAbsResourceInstance(traversal)
	return absResourceInstance.ConfigResource(), diags // TODO maybe we can just use addrs.ParseAbsResource, as traversal might never include indexes
}

func ResolvedImportAddress(expr hcl.Expression) *addrs.AbsResourceInstance {
	var diags hcl.Diagnostics
	traversal, traversalDiags := hcl.AbsTraversalForExpr(expr)
	diags = append(diags, traversalDiags...)
	if diags.HasErrors() {
		return nil
	}

	to, toDiags := addrs.ParseAbsResourceInstance(traversal)
	diags = append(diags, toDiags.ToHCL()...)
	if diags.HasErrors() {
		return nil
	}
	return &to
}
