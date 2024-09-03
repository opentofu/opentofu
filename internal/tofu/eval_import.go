// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package tofu

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/evalchecks"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

func evaluateImportIdExpression(expr hcl.Expression, ctx EvalContext, keyData instances.RepetitionData) (string, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	if expr == nil {
		return "", diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid import id argument",
			Detail:   "The import ID cannot be null.",
			Subject:  nil,
		})
	}

	// evaluate the import ID and take into consideration the for_each key (if exists)
	importIdVal, evalDiags := evaluateExprWithRepetitionData(ctx, expr, cty.String, keyData)
	diags = diags.Append(evalDiags)

	if importIdVal.IsNull() {
		return "", diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid import id argument",
			Detail:   "The import ID cannot be null.",
			Subject:  expr.Range().Ptr(),
		})
	}

	if !importIdVal.IsKnown() {
		return "", diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid import id argument",
			Detail:   `The import block "id" argument depends on resource attributes that cannot be determined until apply, so OpenTofu cannot plan to import this resource.`, // FIXME and what should I do about that?
			Subject:  expr.Range().Ptr(),
			//	Expression:
			//	EvalContext:
			Extra: evalchecks.DiagnosticCausedByUnknown(true),
		})
	}

	if importIdVal.HasMark(marks.Sensitive) {
		return "", diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid import id argument",
			Detail:   "The import ID cannot be sensitive.",
			Subject:  expr.Range().Ptr(),
		})
	}

	var importId string
	err := gocty.FromCtyValue(importIdVal, &importId)
	if err != nil {
		return "", diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid import id argument",
			Detail:   fmt.Sprintf("The import ID value is unsuitable: %s.", err),
			Subject:  expr.Range().Ptr(),
		})
	}

	return importId, diags
}

// evaluateExprWithRepetitionData takes the given HCL expression and evaluates
// it to produce a value, while taking into consideration any repetition key
// (a single combination of each.key and each.value of a for_each argument)
// that should be a part of the scope.
func evaluateExprWithRepetitionData(ctx EvalContext, expr hcl.Expression, wantType cty.Type, keyData instances.RepetitionData) (cty.Value, tfdiags.Diagnostics) {
	scope := ctx.EvaluationScope(nil, nil, keyData)
	return scope.EvalExpr(expr, wantType)
}

// EvaluateImportAddress takes the raw reference expression of the import address
// from the config, and returns the evaluated address addrs.AbsResourceInstance
//
// The implementation is inspired by config.AbsTraversalForImportToExpr, but this time we can evaluate the expression
// in the indexes of expressions. If we encounter a hclsyntax.IndexExpr, we can evaluate the Key expression and create
// an Index Traversal, adding it to the Traverser
func evaluateImportAddress(ctx EvalContext, expr hcl.Expression, keyData instances.RepetitionData) (addrs.AbsResourceInstance, tfdiags.Diagnostics) {
	traversal, diags := traversalForImportExpr(ctx, expr, keyData)
	if diags.HasErrors() {
		return addrs.AbsResourceInstance{}, diags
	}

	return addrs.ParseAbsResourceInstance(traversal)
}

func traversalForImportExpr(ctx EvalContext, expr hcl.Expression, keyData instances.RepetitionData) (hcl.Traversal, tfdiags.Diagnostics) {
	var traversal hcl.Traversal
	var diags tfdiags.Diagnostics

	switch e := expr.(type) {
	case *hclsyntax.IndexExpr:
		t, d := traversalForImportExpr(ctx, e.Collection, keyData)
		diags = diags.Append(d)
		traversal = append(traversal, t...)

		tIndex, dIndex := parseImportIndexKeyExpr(ctx, e.Key, keyData)
		diags = diags.Append(dIndex)
		traversal = append(traversal, tIndex)
	case *hclsyntax.RelativeTraversalExpr:
		t, d := traversalForImportExpr(ctx, e.Source, keyData)
		diags = diags.Append(d)
		traversal = append(traversal, t...)
		traversal = append(traversal, e.Traversal...)
	case *hclsyntax.ScopeTraversalExpr:
		traversal = append(traversal, e.Traversal...)
	default:
		// This should not happen, as it should have failed validation earlier, in config.AbsTraversalForImportToExpr
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid import address expression",
			Detail:   "Import address must be a reference to a resource's address, and only allows for indexing with dynamic keys. For example: module.my_module[expression1].aws_s3_bucket.my_buckets[expression2] for resources inside of modules, or simply aws_s3_bucket.my_bucket for a resource in the root module",
			Subject:  expr.Range().Ptr(),
		})
	}

	return traversal, diags
}

// parseImportIndexKeyExpr parses an expression that is used as a key in an index, of an HCL expression representing an
// import target address, into a traversal of type hcl.TraverseIndex.
// After evaluation, the expression must be known, not null, not sensitive, and must be a string (for_each) or a number
// (count)
func parseImportIndexKeyExpr(ctx EvalContext, expr hcl.Expression, keyData instances.RepetitionData) (hcl.TraverseIndex, tfdiags.Diagnostics) {
	idx := hcl.TraverseIndex{
		SrcRange: expr.Range(),
	}

	// evaluate and take into consideration the for_each key (if exists)
	val, diags := evaluateExprWithRepetitionData(ctx, expr, cty.DynamicPseudoType, keyData)
	if diags.HasErrors() {
		return idx, diags
	}

	if !val.IsKnown() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Import block 'to' address contains an invalid key",
			Detail:   "Import block contained a resource address using an index that will only be known after apply. Please ensure to use expressions that are known at plan time for the index of an import target address",
			Subject:  expr.Range().Ptr(),
		})
		return idx, diags
	}

	if val.IsNull() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Import block 'to' address contains an invalid key",
			Detail:   "Import block contained a resource address using an index which is null. Please ensure the expression for the index is not null",
			Subject:  expr.Range().Ptr(),
		})
		return idx, diags
	}

	if val.Type() != cty.String && val.Type() != cty.Number {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Import block 'to' address contains an invalid key",
			Detail:   "Import block contained a resource address using an index which is not valid for a resource instance (not a string or a number). Please ensure the expression for the index is correct, and returns either a string or a number",
			Subject:  expr.Range().Ptr(),
		})
		return idx, diags
	}

	unmarkedVal, valMarks := val.Unmark()
	if _, sensitive := valMarks[marks.Sensitive]; sensitive {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Import block 'to' address contains an invalid key",
			Detail:   "Import block contained a resource address using an index which is sensitive. Please ensure indexes used in the resource address of an import target are not sensitive",
			Subject:  expr.Range().Ptr(),
		})
	}

	idx.Key = unmarkedVal
	return idx, diags
}
