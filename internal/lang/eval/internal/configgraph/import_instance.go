// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

type ImportInstance struct {
	Addr      addrs.ConfigResource
	DeclRange tfdiags.SourceRange

	// Only used in GenConfig path
	Provider               addrs.Provider
	ProviderInstanceValuer *OnceValuer

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

func (ii *ImportInstance) ProviderInstance(ctx context.Context) (exprs.FromValue[*ProviderInstance], tfdiags.Diagnostics) {
	return ProviderInstanceFromValuer(ctx, ii.ProviderInstanceValuer, ii.Provider, ii.Addr)
}

type ImportStatement struct {
	Addr      addrs.AbsResourceInstance
	DeclRange tfdiags.SourceRange
	ID        string
	Identity  cty.Value
}

// CheckAll implements allChecker.
func (ii *ImportInstance) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	_, diags := ii.Statement(ctx)
	_, moreDiags := ii.ProviderInstance(ctx)
	diags = diags.Append(moreDiags)
	return diags
}

func (ii *ImportInstance) Statement(ctx context.Context) (*ImportStatement, tfdiags.Diagnostics) {
	return ii.result.Do(ctx, ii.CompileImportStatement, &ii.DeclRange, "import statement")
}
