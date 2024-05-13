// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/instances"
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
			Extra: diagnosticCausedByUnknown(true),
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
