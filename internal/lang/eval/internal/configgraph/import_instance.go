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

	CompileImportStatement func(ctx context.Context) (*ImportStatement, tfdiags.Diagnostics)

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
