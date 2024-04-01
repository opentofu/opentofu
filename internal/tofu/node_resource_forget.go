// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"log"

	"github.com/opentofu/opentofu/internal/tfdiags"

	"github.com/opentofu/opentofu/internal/states"
)

// NodeForgetResourceInstance represents a resource instance that is to be
// forgotten from the state.
type NodeForgetResourceInstance struct {
	*NodeAbstractResourceInstance

	// If DeposedKey is set to anything other than states.NotDeposed then
	// this node forgets a deposed object of the associated instance
	// rather than its current object.
	DeposedKey states.DeposedKey
}

var (
	_ GraphNodeModuleInstance      = (*NodeForgetResourceInstance)(nil)
	_ GraphNodeConfigResource      = (*NodeForgetResourceInstance)(nil)
	_ GraphNodeResourceInstance    = (*NodeForgetResourceInstance)(nil)
	_ GraphNodeReferenceable       = (*NodeForgetResourceInstance)(nil)
	_ GraphNodeReferencer          = (*NodeForgetResourceInstance)(nil)
	_ GraphNodeExecutable          = (*NodeForgetResourceInstance)(nil)
	_ GraphNodeProviderConsumer    = (*NodeForgetResourceInstance)(nil)
	_ GraphNodeProvisionerConsumer = (*NodeForgetResourceInstance)(nil)
)

func (n *NodeForgetResourceInstance) Name() string {
	if n.DeposedKey != states.NotDeposed {
		return fmt.Sprintf("%s (forget deposed %s)", n.ResourceInstanceAddr(), n.DeposedKey)
	}
	return n.ResourceInstanceAddr().String() + " (forget)"
}

// GraphNodeExecutable
func (n *NodeForgetResourceInstance) Execute(ctx EvalContext, op walkOperation) (diags tfdiags.Diagnostics) {
	addr := n.ResourceInstanceAddr()

	// Get our state
	is := n.instanceState
	if is == nil {
		log.Printf("[WARN] NodeForgetResourceInstance for %s with no state", addr)
	}

	var state *states.ResourceInstanceObject

	state, readDiags := n.readResourceInstanceState(ctx, addr)
	diags = diags.Append(readDiags)
	if diags.HasErrors() {
		return diags
	}

	// Exit early if the state object is null after reading the state
	if state == nil || state.Value.IsNull() {
		return diags
	}

	contextState := ctx.State()
	contextState.ForgetResourceInstanceAll(n.Addr)

	diags = diags.Append(updateStateHook(ctx))

	return diags
}
