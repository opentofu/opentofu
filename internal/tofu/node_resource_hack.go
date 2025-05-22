package tofu

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type NodeResourceHack struct {
	NodeAbstractResource
}

var (
	_ GraphNodeExecutable    = (*NodeResourceHack)(nil)
	_ GraphNodeReferenceable = (*NodeResourceHack)(nil)
)

func (n *NodeResourceHack) Name() string {
	return n.Addr.String() + " (HACK)"
}

func (n *NodeResourceHack) ReferenceableAddrs() []addrs.Referenceable {
	return []addrs.Referenceable{
		n.Addr.Resource,
	}
}

func (n *NodeResourceHack) Execute(_ context.Context, evalCtx EvalContext, op walkOperation) tfdiags.Diagnostics {
	println("HACK THE PLANET")
	return nil
}
