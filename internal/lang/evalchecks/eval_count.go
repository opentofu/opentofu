// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package evalchecks

import (
	"fmt"
	"runtime"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

type EvaluateFunc func(expr hcl.Expression) (cty.Value, tfdiags.Diagnostics)

// EvaluateCountExpression is our standard mechanism for interpreting an
// expression given for a "count" argument on a resource or a module. This
// should be called during expansion in order to determine the final count
// value.
//
// EvaluateCountExpression differs from EvaluateCountExpressionValue by
// returning an error if the count value is not known, and converting the
// cty.Value to an integer.
//
// If excludableAddr is non-nil then the unknown value error will include
// an additional idea to exclude that address using the -exclude
// planning option to converge over multiple plan/apply rounds.
func EvaluateCountExpression(expr hcl.Expression, ctx EvaluateFunc, excludableAddr addrs.Targetable) (int, tfdiags.Diagnostics) {
	countVal, diags := EvaluateCountExpressionValue(expr, ctx)
	if !countVal.IsKnown() {
		// Currently this is a rather bad outcome from a UX standpoint, since we have
		// no real mechanism to deal with this situation and all we can do is produce
		// an error message.
		// FIXME: In future, implement a built-in mechanism for deferring changes that
		// can't yet be predicted, and use it to guide the user through several
		// plan/apply steps until the desired configuration is eventually reached.

		suggestion := countCommandLineExcludeSuggestion(excludableAddr)
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid count argument",
			Detail:   "The \"count\" value depends on resource attributes that cannot be determined until apply, so OpenTofu cannot predict how many instances will be created.\n\n" + suggestion,
			Subject:  expr.Range().Ptr(),

			// TODO: Also populate Expression and EvalContext in here, but
			// we can't easily do that right now because the hcl.EvalContext
			// (which is not the same as the ctx we have in scope here) is
			// hidden away inside evaluateCountExpressionValue.
			Extra: DiagnosticCausedByUnknown(true),
		})
	}

	if countVal.IsNull() || !countVal.IsKnown() {
		return -1, diags
	}

	count, _ := countVal.AsBigFloat().Int64()
	return int(count), diags
}

// EvaluateCountExpressionValue is like EvaluateCountExpression
// except that it returns a cty.Value which must be a cty.Number and can be
// unknown.
func EvaluateCountExpressionValue(expr hcl.Expression, ctx EvaluateFunc) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	nullCount := cty.NullVal(cty.Number)
	if expr == nil {
		return nullCount, nil
	}

	countVal, countDiags := ctx(expr)
	diags = diags.Append(countDiags)
	if diags.HasErrors() {
		return nullCount, diags
	}

	// Unmark the count value, sensitive values are allowed in count but not for_each,
	// as using it here will not disclose the sensitive value
	countVal, _ = countVal.Unmark()

	switch {
	case countVal.IsNull():
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid count argument",
			Detail:   `The given "count" argument value is null. An integer is required.`,
			Subject:  expr.Range().Ptr(),
		})
		return nullCount, diags

	case !countVal.IsKnown():
		return cty.UnknownVal(cty.Number), diags
	}

	var count int
	err := gocty.FromCtyValue(countVal, &count)
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid count argument",
			Detail:   fmt.Sprintf(`The given "count" argument value is unsuitable: %s.`, err),
			Subject:  expr.Range().Ptr(),
		})
		return nullCount, diags
	}
	if count < 0 {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid count argument",
			Detail:   `The given "count" argument value is unsuitable: must be greater than or equal to zero.`,
			Subject:  expr.Range().Ptr(),
		})
		return nullCount, diags
	}

	return countVal, diags
}

// Returns some English-language text describing a workaround using the -exclude
// planning option to converge over two plan/apply rounds when count has an
// unknown value.
//
// This is intended only for when a count value is too unknown for
// planning to proceed, in [EvaluateCountExpression].
//
// If excludableAddr is non-nil then the message will refer to it directly, giving
// a full copy-pastable command line argument. Otherwise, the message is a generic
// one without any specific address indicated.
func countCommandLineExcludeSuggestion(excludableAddr addrs.Targetable) string {
	// We use an extra indirection here so that we can write tests that make
	// the same assertions on all development platforms.
	return countCommandLineExcludeSuggestionImpl(excludableAddr, runtime.GOOS)
}

func countCommandLineExcludeSuggestionImpl(excludableAddr addrs.Targetable, goos string) string {
	if excludableAddr == nil {
		// We use -target for this case because we can't be sure that the
		// object we're complaining about even has its own addrs.Targetable
		// address, and so the user might need to target only what it depends
		// on instead.
		return `To work around this, use the -target option to first apply only the resources that the count depends on, and then apply normally to converge.`
	}

	return fmt.Sprintf(
		"To work around this, use the planning option -exclude=%s to first apply without this object, and then apply normally to converge.",
		commandLineArgumentsSuggestion([]string{excludableAddr.String()}, goos),
	)
}
