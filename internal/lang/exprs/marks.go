// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exprs

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/ctymarks"
)

// evaluationMark is the type used for a few cty marks we use to help
// distinguish normal unknown value results from those used as placeholders
// for failed evaluation of upstream objects.
type evalResultMark rune

// EvalError is a [cty.Value] mark used on placeholder unknown values returned
// whenever evaluation causes error diagnostics.
//
// This is intended only for use between collaborators in a subsystem where
// everyone is consistently following this convention as a means to avoid
// redundantly reporting downstream consequences of an upstream problem.
// Use [WithoutEvalErrorMarks] at the boundary of such a subsystem so that
// code in other parts of the system does not need to deal with these marks.
//
// In many cases it's okay to ignore this mark and just use the unknown value
// placeholder as normal, letting the mark "infect" the result as necessary,
// but this is here for less common situations where logic _does_ need to handle
// those situations differently.
//
// For example, if a particular language feature treats the mere presence of
// an unknown value as an error then the error-handling logic should first
// check whether the value has this mark and only return the
// unknown-value-related error if not, because the presence of this mark
// suggests that the unknown value is likely caused by another upstream error
// rather than by the module author directly using an unknown value in an
// invalid location.
//
// The expression evaluation mechanisms in this package add this mark
// automatically whenever they generate evaluation error placeholders, but
// it's exposed as an exported symbol so that logic elsewhere that is performing
// non-expression-based evaluation can participate in this marking scheme.
const EvalError = evalResultMark('E')

func (ee evalResultMark) GoString() string {
	return "exprs.EvalError"
}

// AsEvalError returns the given value with [EvalError] applied to it as a mark.
//
// The expression evaluator in this package automatically adds this mark when
// expression evaluation fails, but code elsewhere in the system should use this
// to also treat other kinds of errors as evaluation errors if they are
// returning a placeholder value alongside at least one error diagnostic.
func AsEvalError(v cty.Value) cty.Value {
	return v.Mark(EvalError)
}

// EvalResult is a helper that checks whether the given diags contains errors
// and if so returns the given value with [EvalError] applied to it as a
// mark.
//
// Otherwise it returns the value unmodified. In all cases it returns exactly
// the diagnostics it was given.
//
// This is designed for concise use in a return statement in a function that's
// returning both a value and some diagnostics produced from somewhere else,
// to ensure that the [EvalError] mark still gets applied when appropriate.
func EvalResult(v cty.Value, diags tfdiags.Diagnostics) (cty.Value, tfdiags.Diagnostics) {
	if diags.HasErrors() {
		return AsEvalError(v), diags
	}
	return v, diags
}

// IsEvalError returns true if the given value is directly marked with
// [EvalError], indicating that it's acting as a placeholder for an upstream
// failed evaluation.
//
// This only checks the given value shallowly. Use [HasEvalErrors] instead to
// check whether there are any evaluation error placeholders in nested values.
// For example, a caller that is using [`cty.Value.IsWhollyKnown`] to reject
// a value with unknown values anywhere inside it should prefer to use
// [HasEvalErrors] first to determine if any of the nested unknown values
// might actually be error placeholders.
func IsEvalError(v cty.Value) bool {
	return v.HasMark(EvalError)
}

// HasEvalErrors is like [IsEvalError] except that it visits nested values
// inside the given value recursively and returns true if [EvalError] marks
// are present at any nesting level.
//
// Don't use this except when rejecting values that contain nested unknown
// values in a context where those values are not allowed. If only _shallow_
// unknown values are disallowed then use [IsEvalError] instead to match
// that with only a shallow check for the [EvalError] mark.
func HasEvalErrors(v cty.Value) bool {
	_, marks := v.UnmarkDeep()
	_, marked := marks[EvalError]
	return marked
}

// WithoutEvalErrorMarks returns the given value with any shallow or nested
// [EvalError] marks removed.
//
// Use this at the boundary of a subsystem that uses the evaluation error
// marking scheme internally as an implementation detail, to avoid exposing
// this extra complexity to callers that are merely consuming the finalized
// results.
func WithoutEvalErrorMarks(v cty.Value) cty.Value {
	v, _ = v.WrangleMarksDeep(func(mark any, path cty.Path) (ctymarks.WrangleAction, error) {
		if mark == EvalError {
			return ctymarks.WrangleDrop, nil
		}
		return nil, nil // Leave all other marks alone.
	})
	return v
}
