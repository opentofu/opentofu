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

// Closure is an [Evalable] bound to the [Scope] where it was declared, so
// that the two can travel together.
//
// Closure essentially turns an [Evalable] into a [Valuer], allowing it
// to be evaluated without separately tracking the scope it belongs to.
type Closure struct {
	evalable Evalable
	scope    Scope
}

var _ Valuer = (*Closure)(nil)

// NewClosure associates the given [Evalable] with the given [Scope] so that
// it can be evaluated somewhere else later without losing track of what symbols
// and functions were available where it was declared.
func NewClosure(evalable Evalable, scope Scope) *Closure {
	return &Closure{evalable, scope}
}

// StaticCheckTraversal checks whether the given traversal could apply to any
// possible result from [Closure.Value] on this closure, returning error
// diagnostics if not.
func (c *Closure) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	return StaticCheckTraversal(traversal, c.evalable)
}

// Value returns the result of evaluating the enclosed [Evalable] in the
// enclosed [Scope].
//
// Some [Evalable] implementations block on potentially-time-consuming
// operations, in which case they should respond gracefully to cancellation
// of the given context.
func (c *Closure) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	return Evaluate(ctx, c.evalable, c.scope)
}
