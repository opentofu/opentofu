// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package configgraph

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type CheckRule struct {
	// Condition is the as-yet-unevaluated expression for deciding whether the
	// check is satisfied. Evaluation of this is delayed to allow providing
	// a local scope when the logic in the containing object actually evaluates
	// the check.
	Condition exprs.Evalable

	// ErrorMessageRaw is the as-yet-unevaluated expression for producing an
	// error message when the condition does not pass.
	ErrorMessageRaw exprs.Evalable

	// ParentScope is the scope where the check rule was declared,
	// which might need to be wrapped in a local child scope before actually
	// evaluating the condition and error message.
	ParentScope exprs.Scope

	// EphemeralAllowed indicates whether the condition and error message are
	// allowed to be derived from ephemeral values. If not, the relevant
	// methods will return error diagnostics when ephemeral values emerge.
	EphemeralAllowed bool

	DeclSourceRange tfdiags.SourceRange
}

func (r *CheckRule) Check(ctx context.Context, scopeBuilder exprs.ChildScopeBuilder) (checks.Status, tfdiags.Diagnostics) {
	scope := r.childScope(ctx, scopeBuilder)
	rawResult, diags := exprs.Evaluate(ctx, r.Condition, scope)
	if diags.HasErrors() {
		return checks.StatusError, diags
	}
	rawResult, err := convert.Convert(rawResult, cty.Bool)
	if err == nil && rawResult.IsNull() {
		err = fmt.Errorf("value must not be null")
	}
	if err == nil && rawResult.HasMark(marks.Sensitive) {
		err = fmt.Errorf("must not be derived from a sensitive value")
		// TODO: Also annotate the diagnostic with the "caused by sensitive"
		// annotation, so that the diagnostic renderer can describe where
		// the sensitive values might have come from.
	}
	if err == nil && rawResult.HasMark(marks.Ephemeral) && !r.EphemeralAllowed {
		err = fmt.Errorf("must not be derived from an ephemeral value")
	}
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid check condition",
			Detail:   fmt.Sprintf("Invalid result for check condition expression: %s.", tfdiags.FormatError(err)),
			Subject:  r.Condition.EvalableSourceRange().ToHCL().Ptr(),
		})
		return checks.StatusError, diags
	}
	if !rawResult.IsKnown() {
		return checks.StatusUnknown, diags
	}
	// TODO: Handle "deprecated" marks, adding any deprecation-related
	// diagnostics into diags.
	rawResult, _ = rawResult.Unmark() // marks dealt with above
	if rawResult.True() {
		return checks.StatusPass, diags
	}
	return checks.StatusFail, diags
}

func (r *CheckRule) ErrorMessage(ctx context.Context, scopeBuilder exprs.ChildScopeBuilder) (string, tfdiags.Diagnostics) {
	scope := r.childScope(ctx, scopeBuilder)
	rawResult, diags := exprs.Evaluate(ctx, r.ErrorMessageRaw, scope)
	if diags.HasErrors() {
		return "", diags
	}
	rawResult, err := convert.Convert(rawResult, cty.String)
	if err == nil && rawResult.IsNull() {
		err = fmt.Errorf("value must not be null")
	}
	if err == nil && rawResult.HasMark(marks.Sensitive) {
		err = fmt.Errorf("must not be derived from a sensitive value")
		// TODO: Also annotate the diagnostic with the "caused by sensitive"
		// annotation, so that the diagnostic renderer can describe where
		// the sensitive values might have come from.
	}
	if err == nil && rawResult.HasMark(marks.Ephemeral) && !r.EphemeralAllowed {
		err = fmt.Errorf("must not be derived from an ephemeral value")
	}
	if err == nil && !rawResult.IsKnown() {
		err = fmt.Errorf("derived from value that is not yet known")
		// TODO: Also annotate the diagnostic with the "caused by unknown"
		// annotation, so that the diagnostic renderer can describe where
		// the unknown values might have come from.
	}
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid error message for check",
			Detail:   fmt.Sprintf("Invalid result for error message expression: %s.", tfdiags.FormatError(err)),
			Subject:  r.Condition.EvalableSourceRange().ToHCL().Ptr(),
		})
		return "", diags
	}
	// TODO: Handle "deprecated" marks, adding any deprecation-related
	// diagnostics into diags.
	rawResult, _ = rawResult.Unmark() // marks dealt with above
	return rawResult.AsString(), diags
}

// ConditionRange returns the source range where the condition expression was declared.
func (r *CheckRule) ConditionRange() tfdiags.SourceRange {
	return r.Condition.EvalableSourceRange()
}

// DeclRange returns the source range where this check was declared.
func (r *CheckRule) DeclRange() tfdiags.SourceRange {
	return r.DeclSourceRange
}

func (r *CheckRule) childScope(ctx context.Context, builder exprs.ChildScopeBuilder) exprs.Scope {
	return builder.Build(ctx, r.ParentScope)
}
