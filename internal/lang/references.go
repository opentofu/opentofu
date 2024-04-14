// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/blocktoattr"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// References finds all of the references in the given set of traversals,
// returning diagnostics if any of the traversals cannot be interpreted as a
// reference.
//
// This function does not do any de-duplication of references, since references
// have source location information embedded in them and so any invalid
// references that are duplicated should have errors reported for each
// occurence.
//
// If the returned diagnostics contains errors then the result may be
// incomplete or invalid. Otherwise, the returned slice has one reference per
// given traversal, though it is not guaranteed that the references will
// appear in the same order as the given traversals.
func References(parseRef ParseRef, traversals []hcl.Traversal) ([]*addrs.Reference, tfdiags.Diagnostics) {
	if len(traversals) == 0 {
		return nil, nil
	}

	var diags tfdiags.Diagnostics
	refs := make([]*addrs.Reference, 0, len(traversals))

	for _, traversal := range traversals {
		ref, refDiags := parseRef(traversal)
		diags = diags.Append(refDiags)
		if ref == nil {
			continue
		}
		refs = append(refs, ref)
	}

	return refs, diags
}

// ReferencesInBlock is a helper wrapper around References that first searches
// the given body for traversals, before converting those traversals to
// references.
//
// A block schema must be provided so that this function can determine where in
// the body variables are expected.
func ReferencesInBlock(parseRef ParseRef, body hcl.Body, schema *configschema.Block) ([]*addrs.Reference, tfdiags.Diagnostics) {
	if body == nil {
		return nil, nil
	}

	// We use blocktoattr.ExpandedVariables instead of hcldec.Variables or
	// dynblock.VariablesHCLDec here because when we evaluate a block we'll
	// first apply the dynamic block extension and _then_ the blocktoattr
	// transform, and so blocktoattr.ExpandedVariables takes into account
	// both of those transforms when it analyzes the body to ensure we find
	// all of the references as if they'd already moved into their final
	// locations, even though we can't expand dynamic blocks yet until we
	// already know which variables are required.
	//
	// The set of cases we want to detect here is covered by the tests for
	// the plan graph builder in the main 'tofu' package, since it's
	// in a better position to test this due to having mock providers etc
	// available.
	traversals := blocktoattr.ExpandedVariables(body, schema)

	funcs, funcDiags := FunctionsInBlock(body, schema)
	traversals = append(traversals, filterFuncTraversals(funcs)...)

	refs, diags := References(parseRef, traversals)
	return refs, diags.Append(funcDiags)
}

func FunctionsInBlock(body hcl.Body, schema *configschema.Block) (funcs []hcl.Traversal, diags tfdiags.Diagnostics) {
	// TODO this might not properly handle dynamic blocks!

	givenRawSchema := hcldec.ImpliedSchema(schema.DecoderSpec())
	content, _, cDiags := body.PartialContent(givenRawSchema)
	diags = diags.Append(cDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	for _, attr := range content.Attributes {
		aFns, aDiags := FunctionsInExpr(attr.Expr)
		diags = diags.Append(aDiags)
		if diags.HasErrors() {
			return nil, diags
		}
		funcs = append(funcs, aFns...)
	}

	for _, block := range content.Blocks {
		aFns, aDiags := FunctionsInBlock(block.Body, &schema.BlockTypes[block.Type].Block)
		diags = diags.Append(aDiags)
		if diags.HasErrors() {
			return nil, diags
		}
		funcs = append(funcs, aFns...)
	}

	return funcs, diags
}

// ReferencesInExpr is a helper wrapper around References that first searches
// the given expression for traversals, before converting those traversals
// to references.
func ReferencesInExpr(parseRef ParseRef, expr hcl.Expression) ([]*addrs.Reference, tfdiags.Diagnostics) {
	if expr == nil {
		return nil, nil
	}
	traversals := expr.Variables()

	funcs, funcDiags := FunctionsInExpr(expr)
	traversals = append(traversals, filterFuncTraversals(funcs)...)

	refs, diags := References(parseRef, traversals)
	return refs, diags.Append(funcDiags)
}

func FunctionsInExpr(expr hcl.Expression) ([]hcl.Traversal, tfdiags.Diagnostics) {
	if expr == nil {
		return nil, nil
	}

	var diags tfdiags.Diagnostics
	walker := make(fnWalker, 0)
	diags = diags.Append(hclsyntax.Walk(expr.(hclsyntax.Expression), &walker))
	return walker, diags
}

type fnWalker []hcl.Traversal

func (w *fnWalker) Enter(node hclsyntax.Node) hcl.Diagnostics {
	if fn, ok := node.(*hclsyntax.FunctionCallExpr); ok {
		sp := strings.Split(fn.Name, "::")

		t := hcl.Traversal{hcl.TraverseRoot{
			Name:     sp[0],
			SrcRange: fn.NameRange,
		}}
		for _, part := range sp[1:] {
			t = append(t, hcl.TraverseAttr{
				Name:     part,
				SrcRange: fn.NameRange,
			})
		}

		*w = append(*w, t)
	}
	return nil
}
func (w *fnWalker) Exit(node hclsyntax.Node) hcl.Diagnostics {
	return nil
}

func filterFuncTraversals(fns []hcl.Traversal) (traversals []hcl.Traversal) {
	for _, fn := range fns {
		if len(fn) == 3 || len(fn) == 4 {
			traversals = append(traversals, fn)
		}
	}
	return traversals
}
