// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package configgraph

import (
	"context"
	"fmt"
	"iter"
	"maps"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// CheckRule represents an author-defined condition that must be true and
// an error message to return if it isn't true.
//
// In many cases the result of a check rule depends on some other value
// in a local scope, in which case the type that the check rules belong
// to must include a callback function that returns check rules based
// on that result rather than predefined inline check rules. The [exprs.Valuer]
// objects in a [CheckRule] must be pre-bound to whatever local scope is
// appropriate for the context where they are declared.
//
// If you have an [iter.Seq] of [*CheckRule], or something that you can
// conveniently use as one, [checkAllRules] is a useful way to visit all
// of them and react consistently to their results.
type CheckRule struct {
	// ConditionValuer produces the boolean result which determines whether
	// the check passes. The result should be of type [cty.Bool] and should
	// be [cty.True] if the condition is satisfied or [cty.False] if it is
	// not.
	//
	// The valuer is also allowed to return an unknown value if it isn't
	// yet possible to decide whether the condition is satisfied. Null
	// values are not allowed and will cause the check to fail with "error"
	// status.
	ConditionValuer exprs.Valuer

	// ErrorMessageValuer returns a string value containing an error message
	// that should be used when the condition is not satified.
	//
	// The result is required to be known and non-null. If this valuer
	// fails to evaluate with error diagnostics then those error diagnostics
	// will be returned along with a generic error message and the check
	// will fail with the "error" status.
	//
	// This valuer is used only when [ConditionValuer] returns [cty.False].
	ErrorMessageValuer exprs.Valuer

	// DeclSourceRange is a source range that the module author would consider
	// to represent the declaration of this check rule, for use in error
	// messages that describe which rule was responsible for detecting a
	// failure.
	DeclSourceRange tfdiags.SourceRange
}

func (r *CheckRule) Check(ctx context.Context) (checks.Status, cty.ValueMarks, tfdiags.Diagnostics) {
	rawResult, diags := r.ConditionValuer.Value(ctx)
	rawResult, resultMarks := rawResult.Unmark()
	if diags.HasErrors() {
		resultMarks[exprs.EvalError] = struct{}{}
		return checks.StatusError, resultMarks, diags
	}
	rawResult, err := convert.Convert(rawResult, cty.Bool)
	if err == nil && rawResult.IsNull() {
		err = fmt.Errorf("value must not be null")
	}
	if _, sensitive := resultMarks[marks.Sensitive]; err == nil && sensitive {
		err = fmt.Errorf("must not be derived from a sensitive value")
		// TODO: Also annotate the diagnostic with the "caused by sensitive"
		// annotation, so that the diagnostic renderer can describe where
		// the sensitive values might have come from.
	}
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid check condition",
			Detail:   fmt.Sprintf("Invalid result for check condition expression: %s.", tfdiags.FormatError(err)),
			Subject:  r.ConditionValuer.ValueSourceRange().ToHCL().Ptr(),
		})
		resultMarks[exprs.EvalError] = struct{}{}
		return checks.StatusError, resultMarks, diags
	}
	if !rawResult.IsKnown() {
		return checks.StatusUnknown, resultMarks, diags
	}
	if rawResult.True() {
		return checks.StatusPass, resultMarks, diags
	}
	return checks.StatusFail, resultMarks, diags
}

func (r *CheckRule) ErrorMessage(ctx context.Context) (string, tfdiags.Diagnostics) {
	rawResult, diags := r.ErrorMessageValuer.Value(ctx)
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
			Subject:  r.ErrorMessageValuer.ValueSourceRange().ToHCL().Ptr(),
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
	if condRng := r.ConditionValuer.ValueSourceRange(); condRng != nil {
		return *condRng
	}
	// If the condition valuer doesn't have its own range then we'll
	// use the check rule's overall declaration range as a fallback.
	return r.DeclRange()
}

// DeclRange returns the source range where this check was declared.
func (r *CheckRule) DeclRange() tfdiags.SourceRange {
	return r.DeclSourceRange
}

// CheckAllRules deals with the boilerplate of evaluating a series of
// [CheckRule] objects and reacting to their results.
//
// Evaluates each rule in turn and then calls handleResult for each one,
// passing the final status, and the error message if and only if the
// status is [checks.StatusFail].
//
// handleResult may choose to return diagnostics to add to the final
// aggregate set of diagnostics, but should typically add error diagnostics
// only if the status is [checks.StatusFail] because check rules generate
// their own error diagnostics for totally-invalid cases that yield
// [checks.StatusError].
//
// The results are a set of all of the cty marks on the condition results
// of the rules and an aggregate set of diagnostics mixing any
// automatically-generated usage errors with failure-related diagonstics
// returned by handleResult. A caller should typically transfer all of
// the returned marks to whatever values were being checked to reflect
// that the final value was effectively "derived from" the check results.
func CheckAllRules(ctx context.Context, rules iter.Seq[*CheckRule], handleResult func(ruleDeclRange tfdiags.SourceRange, status checks.Status, errMsg string) tfdiags.Diagnostics) (cty.ValueMarks, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	marks := make(cty.ValueMarks)
	for rule := range rules {
		status, moreMarks, moreDiags := rule.Check(ctx)
		maps.Copy(marks, moreMarks)
		diags = diags.Append(moreDiags)
		var errMsg string // empty unless StatusFail
		if status == checks.StatusFail {
			errMsg, moreDiags = rule.ErrorMessage(ctx)
			diags = diags.Append(moreDiags)
		}
		moreDiags = handleResult(rule.DeclSourceRange, status, errMsg)
		diags = diags.Append(moreDiags)
	}
	return marks, diags
}
