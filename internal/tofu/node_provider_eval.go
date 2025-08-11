// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"

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
}

var _ GraphNodeExecutable = (*NodeEvalableProvider)(nil)

// GraphNodeExecutable
func (n *NodeEvalableProvider) Execute(ctx context.Context, evalCtx EvalContext, op walkOperation) (diags tfdiags.Diagnostics) {
	provider, err := evalCtx.InitProvider(ctx, n.Addr, addrs.NoKey)

	diags = diags.Append(err)
	return diags.Append(n.InitEvalableProvider(ctx, evalCtx, provider))
}

// InitEvalableProvider fetches its schema, builds it, and adds it
// to the context.
func (n *NodeEvalableProvider) InitEvalableProvider(ctx context.Context, evalCtx EvalContext, provider providers.Interface) tfdiags.Diagnostics {
	providerKey := addrs.NoKey
	config := n.ProviderConfig()
	configBody := buildProviderConfig(ctx, evalCtx, n.Addr, config)

	schemaResp := provider.GetProviderSchema(ctx)
	return schemaResp.Diagnostics.InConfigBody(configBody, n.Addr.InstanceString(providerKey))
}
