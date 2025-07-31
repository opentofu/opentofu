// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	hcljson "github.com/hashicorp/hcl/v2/json"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/hcl2shim"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Import struct {
	ID hcl.Expression

	// To is the address HCL expression given in the `import` block configuration.
	// It supports the following address formats:
	// - aws_s3_bucket.my_bucket
	// - module.my_module.aws_s3_bucket.my_bucket
	// - aws_s3_bucket.my_bucket["static_key"]
	// - module.my_module[0].aws_s3_bucket.my_buckets["static_key"]
	// - aws_s3_bucket.my_bucket[expression]
	// - module.my_module[expression].aws_s3_bucket.my_buckets[expression]
	// A dynamic instance key supports a dynamic expression like - a variable, a local, a condition (for example,
	//  ternary), a resource block attribute, a data block attribute, etc.
	To hcl.Expression
	// StaticTo is the corresponding resource and module that the address is referring to. When decoding, as long
	// as the `to` field is in the accepted format, we could determine the actual modules and resource that the
	// address represents. However, we do not yet know for certain what module instance and resource instance this
	// address refers to. So, Static import is mainly used to figure out the Module and Resource, and Provider of the
	// import target resource
	// If we could not determine the StaticTo when decoding the block, then the address is in an unacceptable format
	StaticTo addrs.ConfigResource
	// ResolvedTo will be a reference to the resource instance of the import target, if it can be resolved when decoding
	// the `import` block. If the `to` field does not represent a static address
	// (for example: module.my_module[var.var1].aws_s3_bucket.bucket), then this will be nil.
	// However, if the address is static and can be fully resolved at decode time
	// (for example: module.my_module[2].aws_s3_bucket.bucket), then this will be a reference to the resource instance's
	// address
	// Mainly used for early validations on the import block address, for example making sure there are no duplicate
	// import blocks targeting the same resource
	ResolvedTo *addrs.AbsResourceInstance

	ForEach hcl.Expression

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
		toExpr := attr.Expr
		// Since we are manually parsing the 'to' argument, we need to specially
		// handle json configs, in which case the values will be json strings
		// rather than hcl
		isJSON := hcljson.IsJSONExpression(attr.Expr)

		if isJSON {
			convertedExpr, convertDiags := hcl2shim.ConvertJSONExpressionToHCL(toExpr)
			diags = append(diags, convertDiags...)

			if diags.HasErrors() {
				return imp, diags
			}

			toExpr = convertedExpr
		}

		imp.To = toExpr
		staticAddress, addressDiags := staticImportAddress(toExpr)
		diags = append(diags, addressDiags.ToHCL()...)

		// Exit early if there are issues resolving the static address part. We wouldn't be able to validate the provider in such a case
		if addressDiags.HasErrors() {
			return imp, diags
		}
		imp.StaticTo = staticAddress

		imp.ResolvedTo = resolvedImportAddress(imp.To)
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

	if attr, exists := content.Attributes["for_each"]; exists {
		imp.ForEach = attr.Expr
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
		{
			Name: "for_each",
		},
	},
}

// absTraversalForImportToExpr returns a static traversal of an import block's "to" field.
// It is inspired by hcl.AbsTraversalForExpr and by tofu.triggersExprToTraversal
// The use-case here is different - we want to also allow for hclsyntax.IndexExpr to be allowed,
// but we don't really care about the key part of it. We just want a traversal that could be converted to an address
// of a resource, so we could determine the module + resource + provider
//
// Currently, there are 4 types of HCL expressions that support AsTraversal:
// - hclsyntax.ScopeTraversalExpr - Simply returns the Traversal. Same for our use-case here
// - hclsyntax.RelativeTraversalExpr - Calculates hcl.AbsTraversalForExpr for the Source, and adds the Traversal to it. Same here, with absTraversalForImportToExpr instead
// - hclsyntax.LiteralValueExpr - Mainly for null/false/true values. Not relevant in our use-case, as it's could not really be part of a reference (unless it is inside of an index, which is irrelevant here anyway)
// - hclsyntax.ObjectConsKeyExpr - Not relevant here
//
// In addition to these, we need to also support hclsyntax.IndexExpr. For it - we do not care about what's in the index.
// We need only know the traversal parts of it the "Collection", as the index doesn't affect which resource/module this is
func absTraversalForImportToExpr(expr hcl.Expression) (traversal hcl.Traversal, diags tfdiags.Diagnostics) {
	switch e := expr.(type) {
	case *hclsyntax.IndexExpr:
		t, d := absTraversalForImportToExpr(e.Collection)
		diags = diags.Append(d)
		traversal = append(traversal, t...)
	case *hclsyntax.RelativeTraversalExpr:
		t, d := absTraversalForImportToExpr(e.Source)
		diags = diags.Append(d)
		traversal = append(traversal, t...)
		traversal = append(traversal, e.Traversal...)
	case *hclsyntax.ScopeTraversalExpr:
		traversal = append(traversal, e.Traversal...)
	default:
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid import address expression",
			Detail:   "Import address must be a reference to a resource's address, and only allows for indexing with dynamic keys. For example: module.my_module[expression1].aws_s3_bucket.my_buckets[expression2] for resources inside of modules, or simply aws_s3_bucket.my_bucket for a resource in the root module",
			Subject:  expr.Range().Ptr(),
		})
	}
	return
}

// staticImportAddress returns an addrs.ConfigResource representing the module and resource of the import target.
// If the address is of an unacceptable format, the function will return error diags
func staticImportAddress(expr hcl.Expression) (addrs.ConfigResource, tfdiags.Diagnostics) {
	traversal, diags := absTraversalForImportToExpr(expr)
	if diags.HasErrors() {
		return addrs.ConfigResource{}, diags
	}

	absResourceInstance, diags := addrs.ParseAbsResourceInstance(traversal)
	return absResourceInstance.ConfigResource(), diags
}

// resolvedImportAddress attempts to find the resource instance of the import target, if possible.
// Here, we attempt to resolve the address as though it is a static absolute traversal, if that's possible.
// This would only be possible if the `import` block's "to" field does not rely on any data that is dynamic
func resolvedImportAddress(expr hcl.Expression) *addrs.AbsResourceInstance {
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
