// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// NodeEvalableProvider represents a provider during an "eval" walk.
// This special provider node type just initializes a provider and
// fetches its schema, without configuring it or otherwise interacting
// with it.
type NodeEvalableProvider struct {
	*NodeAbstractProvider

	instance providers.Configured
}

var _ GraphNodeExecutable = (*NodeEvalableProvider)(nil)
var _ GraphNodeProvider = (*NodeEvalableProvider)(nil) // Partial, see NodeAbstractProvider

// GraphNodeProvider
func (n *NodeEvalableProvider) Instance(key addrs.InstanceKey) (providers.Configured, error) {
	if key != addrs.NoKey {
		return nil, fmt.Errorf("unexpected key %s passed to NodeEvalableProvider.Instance for %s", key, n.Addr)
	}
	if n.instance == nil {
		return nil, fmt.Errorf("bug: provider %s not yet initialized or encountered an error during initialization", n.Addr)
	}
	return n.instance, nil
}

// GraphNodeProvider
func (n *NodeEvalableProvider) Close(ctx context.Context) error {
	if n.instance == nil {
		return fmt.Errorf("bug: provider %s not yet initialized or encountered an error during initialization", n.Addr)
	}
	return n.instance.Close(ctx)
}

// GraphNodeExecutable
func (n *NodeEvalableProvider) Execute(ctx context.Context, evalCtx EvalContext, op walkOperation) (diags tfdiags.Diagnostics) {
	n.instance, diags = evalCtx.Providers().NewProvider(ctx, n.Addr.Provider)
	return diags
}
