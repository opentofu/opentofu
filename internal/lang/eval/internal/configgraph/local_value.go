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

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type LocalValue struct {
	Addr     addrs.AbsLocalValue
	RawValue *OnceValuer
}

var _ exprs.Valuer = (*LocalValue)(nil)

// StaticCheckTraversal implements exprs.Valuer.
func (l *LocalValue) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	return l.RawValue.StaticCheckTraversal(traversal)
}

// Value implements exprs.Valuer.
func (l *LocalValue) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	// There aren't really any special rules for a local value: it just
	// allows authors to associate a value with a name so they can reuse
	// it multiple places.
	return l.RawValue.Value(ctx)
}

// ValueSourceRange implements exprs.Valuer.
func (l *LocalValue) ValueSourceRange() *tfdiags.SourceRange {
	return l.RawValue.ValueSourceRange()
}

// CheckAll implements allChecker.
func (l *LocalValue) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	var cg CheckGroup
	cg.CheckValuer(ctx, l) // We just check our overall Valuer method because it aggregates everything
	return cg.Complete(ctx)
}

func (l *LocalValue) AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo)) {
	announce(l.RawValue.RequestID(), grapheval.RequestInfo{
		// FIXME: Have the "compiler" in package eval put an
		// addrs.AbsLocalValue in here so we can generate a useful name.
		Name:        l.Addr.String(),
		SourceRange: l.RawValue.ValueSourceRange(),
	})
}
