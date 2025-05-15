// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// NodeEvalableProvider represents a provider during an "eval" walk.
// This special provider node type just initializes a provider and
// fetches its schema, without configuring it or otherwise interacting
// with it.
type NodeEvalableProvider struct {
	*NodeAbstractProvider
}

var _ GraphNodeExecutable = (*NodeEvalableProvider)(nil)

// GraphNodeExecutable
func (n *NodeEvalableProvider) Execute(_ context.Context, evalCtx EvalContext, op walkOperation) (diags tfdiags.Diagnostics) {
	_, err := evalCtx.InitProvider(n.Addr, addrs.NoKey)
	return diags.Append(err)
}
