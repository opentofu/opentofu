// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

type ImportInstance struct {
	DeclRange tfdiags.SourceRange

	// CompileImportStatement is a callback function provided by whatever
	// compiled this [ImportInstance] object that knows how to produce a compiled
	// [ImportStatement] object.
	//
	// This indirection allows the caller to take into account weird
	// special case handling for import block fields that don't fit the typical
	// compiler patterns.
	CompileImportStatement func(ctx context.Context) (*ImportStatement, tfdiags.Diagnostics)

	// result helps us to memoize the result from [CompileImportStatement]
	// so that we'll definitely return a consistent value to every call.
	result grapheval.Once[*ImportStatement]
}

type ImportStatement struct {
	Addr      addrs.AbsResourceInstance
	DeclRange tfdiags.SourceRange
	ID        string
	Identity  cty.Value
}

// CheckAll implements allChecker.
func (i *ImportInstance) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	_, diags := i.Statement(ctx)
	return diags
}

func (i *ImportInstance) Statement(ctx context.Context) (*ImportStatement, tfdiags.Diagnostics) {
	return i.result.Do(ctx, i.CompileImportStatement, &i.DeclRange, "import statement")
}
