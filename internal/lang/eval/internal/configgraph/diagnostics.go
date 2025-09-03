// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func MaybeHCLSourceRange(maybeRng *tfdiags.SourceRange) *hcl.Range {
	if maybeRng == nil {
		return nil
	}
	return maybeRng.ToHCL().Ptr()
}

// diagsHandledElsewhere takes the result of a function that returns a value
// along with diagnostics and discards the diagnostics, returning the value
// possibly marked with [exprs.EvalError].
//
// Any use of this function should typically have a nearby comment justifying
// why it's okay to use. The remainder of this doc comment explains _in general_
// what kinds of situations are valid for using this function.
//
// It only makes sense to use this with return values from functions that
// guarantee to return a useful placeholder valueeven when they return error
// diagnostics. For example, [exprs.Valuer] and [exprs.Evalable] implementations
// are both expected to return an unknown value placeholder suitable for use
// in downstream expressions even when they encounter errors.
//
// This is here to model our pattern where the evaluation of a particular
// object or expression should only return diagnostics directly related to
// that object or expression, and should not incorporate diagnostics related
// to other objects depended on indirectly. This pattern is under the assumption
// that all objects will be visited directly during normal processing and
// so will get an opportunity to return their own diagnostics at that point.
//
// The expression evaluator in package exprs automatically handles the most
// common case of this pattern where an expression includes a reference to
// something which cannot generate its own value without encountering
// diagnostics; those indirect diagnostics are already disccarded during
// the preparation of the expression's evaluation context. Explicit use of
// this function is therefore needed only for situations where direct logic
// is in some sense behaving _like_ expression evaluation -- combining
// representations of multiple objects from elsewhere into a larger overall
// result -- but without going through the expression evaluation machinery
// to do it.
//
// The body of this function is straightforward but we call it intentionally so
// that uses of it are clearly connected with this documentation.
func diagsHandledElsewhere(v cty.Value, diags tfdiags.Diagnostics) cty.Value {
	if diags.HasErrors() {
		// If the value was derived from a failing expression evaluation then
		// this mark would probably already be present anyway, but we'll
		// handle it again here just to help get consistent behavior when
		// we're building values with hand-written logic instead of by
		// normal expression evaluation.
		v = v.Mark(exprs.EvalError)
	}
	return v
}
