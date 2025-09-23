// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package evalchecks

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// EvaluateEnabledExpression evaluates an expression assigned to an "enabled"
// meta-argument and returns either its boolean result or errors describing
// why such a result cannot be evaluated.
func EvaluateEnabledExpression(expr hcl.Expression, hclCtxFunc ContextFunc) (bool, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	if expr == nil {
		return false, diags
	}

	refs, refsDiags := lang.ReferencesInExpr(addrs.ParseRef, expr)
	diags = diags.Append(refsDiags)
	var hclCtx *hcl.EvalContext
	hclCtx, refsDiags = hclCtxFunc(refs)
	diags = diags.Append(refsDiags)
	if diags.HasErrors() { // Can't continue if we don't even have a valid scope
		return false, diags
	}

	rawEnabledVal, enabledDiags := expr.Value(hclCtx)
	diags = diags.Append(enabledDiags)

	if rawEnabledVal.HasMark(marks.Sensitive) {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid enabled argument",
			Detail:      "Sensitive values, or values derived from sensitive values, cannot be used in \"enabled\" arguments. If used, the sensitive value would be exposed by whether an instance is present.",
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: hclCtx,
			Extra:       DiagnosticCausedByUnknown(true),
		})
	}

	if rawEnabledVal.HasMark(marks.Ephemeral) {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid enabled argument",
			Detail:      `The given "enabled" argument value is unsuitable: the given value is ephemeral.`,
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: hclCtx,
		})
	}

	if rawEnabledVal.IsNull() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid enabled argument",
			Detail:      `The given "enabled" argument value is unsuitable: the given value is null.`,
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: hclCtx,
		})
	}
	if !rawEnabledVal.IsKnown() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid enabled argument",
			Detail:      `The given "enabled" argument value is derived from a value that won't be known until the apply phase, so OpenTofu cannot determine whether an instance of this object is declared or not.`,
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: hclCtx,
			Extra:       DiagnosticCausedByUnknown(true),
		})
	}

	if diags.HasErrors() {
		return false, diags
	}

	enabledVal, err := convert.Convert(rawEnabledVal, cty.Bool)
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid enabled argument",
			Detail:      fmt.Sprintf(`The given "enabled" argument value is unsuitable: %s.`, err),
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: hclCtx,
		})
	}

	if diags.HasErrors() {
		return false, diags
	}

	// If we get here then we've eliminated all of the reasons why the
	// following could potentially panic.
	return enabledVal.True(), diags
}
