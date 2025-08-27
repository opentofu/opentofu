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
}
