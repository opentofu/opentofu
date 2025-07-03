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

type GraphNodeCloseableResource interface {
	closeableSigil()
}

// nodeCloseableResource is meant to just call the resourceCloser callback right before closing the providers.
// This is done this way strictly because all the information that it's needed to successfully handle ephemeral
// resources closing is in the node type that also opens it.
type nodeCloseableResource struct {
	cb   resourceCloser
	Addr addrs.ConfigResource
}

var (
	_ GraphNodeCloseableResource = (*nodeCloseableResource)(nil)
)

func (n *nodeCloseableResource) Name() string {
	return n.Addr.String() + " (close)"
}

func (n *nodeCloseableResource) Execute(_ context.Context, _ EvalContext, _ walkOperation) (diags tfdiags.Diagnostics) {
	return n.cb()
}

func (n *nodeCloseableResource) closeableSigil() {
}
