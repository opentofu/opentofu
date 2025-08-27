// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exprs

import (
	"context"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Valuer is implemented by objects that can be directly represented by a
// [cty.Value].
//
// This is a similar idea to [Evalable], but with a subtle difference: a
// [Valuer] represents a value directly, whereas an [Evalable] represents an
// expression that can be evaluated to produce a value. In practice some
// [Valuer]s will produce their result by evaluating an [Evalable], but
// that's an implementation detail that the consumer of this interface is
// not aware of.
//
// An [Evalable] can be turned into a [Valuer] by associating it with a
// [Scope], using [NewClosure].
type Valuer interface {
	// Value returns the [cty.Value] representation of this object.
	//
	// This method takes a [context.Context] because some implementations
	// may internally block on the completion of a potentially-time-consuming
	// operation, in which case they should respond gracefully to the
	// cancellation or deadline of the given context.
	//
	// If Value returns diagnostics then it must also return a suitable
	// placeholder value that could be use for downstream expression evaluation
	// despite the error. Returning [cty.DynamicVal] is acceptable if all else
	// fails, but returning an unknown value with a more specific type
	// constraint can give more opportunities to proactively detect downstream
	// errors in a single evaluation pass.
	Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics)

	// StaticCheckTraversal checks whether the given relative traversal would
	// be valid to apply to all values that the ExprValue method could possibly
	// return without doing any expensive work to finalize that value, returning
	// diagnostics describing any problems.
	//
	// A typical implementation of this would be to check whether the
	// traversal makes sense for whatever type constraint applies to all of
	// the values that could possibly be returned. However, it's valid to
	// just immediately return no diagnostics if it's impossible to predict
	// anything about the value, in which case errors will be caught dynamically
	// once the value has been finalized.
	//
	// This function should only return errors that should not be interceptable
	// by the "try" or "can" functions in the OpenTofu language.
	StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics

	// ValueSourceRange returns an optional source range where this value (or an
	// expression that produced it) was declared in configuration.
	//
	// Returns nil for a valuer that does not come from configuration.
	ValueSourceRange() *tfdiags.SourceRange
}

// ConstantValuer returns a [Valuer] that always succeeds and returns exactly
// the value given.
func ConstantValuer(v cty.Value) Valuer {
	return constantValuer{v, nil}
}

// ConstantValuerWithSourceRange is like [ConstantValuer] except that the
// result will also claim to have originated in the configuration at whatever
// source range is given.
func ConstantValuerWithSourceRange(v cty.Value, rng tfdiags.SourceRange) Valuer {
	return constantValuer{v, &rng}
}

// ForcedErrorValuer returns a [Valuer] that always fails with [cty.DynamicVal]
// as its placeholder result and with the given diagnostics, which must include
// at least one error or this function will panic.
//
// This is primarily intended for unit testing purposes for creating
// placeholders for upstream objects that have failed, but might also be useful
// sometimes for handling unusual situations in "real" code.
func ForcedErrorValuer(diags tfdiags.Diagnostics) Valuer {
	if !diags.HasErrors() {
		panic("ForcedErrorHandler without any error diagnostics")
	}
	return forcedErrorValuer{diags}
}

type constantValuer struct {
	v           cty.Value
	sourceRange *tfdiags.SourceRange
}

// StaticCheckTraversal implements Valuer.
func (c constantValuer) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	_, hclDiags := traversal.TraverseRel(c.v)
	diags = diags.Append(hclDiags)
	return diags
}

// Value implements Valuer.
func (c constantValuer) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	return c.v, nil
}

// ValueSourceRange implements Valuer.
func (c constantValuer) ValueSourceRange() *tfdiags.SourceRange {
	return c.sourceRange
}

type forcedErrorValuer struct {
	diags tfdiags.Diagnostics
}

// StaticCheckTraversal implements Valuer.
func (f forcedErrorValuer) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	// This never actually produces a successful result, so there's nothing
	// to check against and we'll just wait until Value is called to return
	// our predefined diagnostics.
	return nil
}

// Value implements Valuer.
func (f forcedErrorValuer) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	return cty.DynamicVal, f.diags
}

// ValueSourceRange implements Valuer.
func (f forcedErrorValuer) ValueSourceRange() *tfdiags.SourceRange {
	return nil
}
