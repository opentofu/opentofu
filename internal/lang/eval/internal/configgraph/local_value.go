// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"

	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

type LocalValue struct {
	RawValue exprs.Valuer
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
