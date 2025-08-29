// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ValuerOnce wraps the given [exprs.Valuer] so that the underlying [Value]
// method will be called only once and reused for all future calls.
//
// Calls to Value on the result must be made with a context derived from
// one produced by [grapheval.ContextWithWorker], which is then used to
// track and report dependency cycles. If the given context is not so
// annotated then Value will immediately panic.
//
// The StaticCheckTraversal method is _not_ wrapped and so should be a
// relatively cheap operation as usual and must not interact (directly or
// indirectly) with any grapheval helpers.
func ValuerOnce(valuer exprs.Valuer) *OnceValuer {
	return &OnceValuer{inner: valuer}
}

type OnceValuer struct {
	once  grapheval.Once[cty.Value]
	inner exprs.Valuer
}

// StaticCheckTraversal implements exprs.Valuer.
func (v *OnceValuer) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	return v.inner.StaticCheckTraversal(traversal)
}

// Value implements exprs.Valuer.
func (v *OnceValuer) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	return v.once.Do(ctx, func(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
		return v.inner.Value(ctx)
	})
}

// ValueSourceRange implements exprs.Valuer.
func (v *OnceValuer) ValueSourceRange() *tfdiags.SourceRange {
	return v.inner.ValueSourceRange()
}

// RequestID returns the workgraph package's tracking identifier for the
// request to return the value, or [workgraph.NoRequest] if nobody has
// called the Value method yet.
func (v *OnceValuer) RequestID() workgraph.RequestID {
	return v.once.RequestID()
}
