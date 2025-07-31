// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"log"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// NodeDestroyableDataResourceInstance represents a resource that is "destroyable":
// it is ready to be destroyed.
type NodeDestroyableDataResourceInstance struct {
	*NodeAbstractResourceInstance
}

var (
	_ GraphNodeExecutable = (*NodeDestroyableDataResourceInstance)(nil)
)

// GraphNodeExecutable
func (n *NodeDestroyableDataResourceInstance) Execute(ctx context.Context, evalCtx EvalContext, op walkOperation) tfdiags.Diagnostics {
	log.Printf("[TRACE] NodeDestroyableDataResourceInstance: removing state object for %s", n.Addr)
	evalCtx.State().SetResourceInstanceCurrent(n.Addr, nil, n.ResolvedProvider.ProviderConfig, nil)
	return nil
}
