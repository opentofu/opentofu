// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"log"
	"sync"

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
	cbs  []resourceCloser
	Addr addrs.ConfigResource
}

var (
	_ GraphNodeCloseableResource = (*nodeCloseableResource)(nil)
)

func (n *nodeCloseableResource) Name() string {
	return n.Addr.String() + " (close)"
}

func (n *nodeCloseableResource) Execute(_ context.Context, _ EvalContext, _ walkOperation) (diags tfdiags.Diagnostics) {
	var wg sync.WaitGroup
	diagsCh := make(chan tfdiags.Diagnostics, len(n.cbs))
	log.Printf("[TRACE] nodeCloseableResource - scheduling %d closing operations for of ephemeral resource %s", len(n.cbs), n.Addr.String())
	// NOTE: since go v1.22 there is no need to copy the loop variable.
	for _, cb := range n.cbs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			diagsCh <- cb()
		}()
	}
	wg.Wait()
	close(diagsCh)
	for d := range diagsCh {
		diags = diags.Append(d)
	}
	return diags
}

func (n *nodeCloseableResource) closeableSigil() {
}
