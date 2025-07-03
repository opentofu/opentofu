// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/dag"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// resourceCloser is the definition of the function that needs to exist on the node that wants to release resources related to the
// resource type.
type resourceCloser func() tfdiags.Diagnostics

// closableResource needs to be implemented by whatever resource wants to be closed before closing the associated
// provider that is handling that type.
type closableResource interface {
	GraphNodeModulePath
	ResourceAddr() addrs.ConfigResource
	Name() string
	Close() tfdiags.Diagnostics
}

// CloseableResourceTransformer is adding new nodes into the graph, responsible with closing specific resource types.
// Right now, there is only the ephemeral resources that are requiring such an action.
type CloseableResourceTransformer struct {
	skip bool
}

func (t *CloseableResourceTransformer) Transform(_ context.Context, g *Graph) error {
	if t.skip {
		return nil
	}
	pm := closeableResourcesVertexMap(g)
	cpm := make(map[string]*nodeCloseableResource)

	for _, rn := range pm {
		key := rn.ResourceAddr().String()

		// check if we already generated a closing node for that particular resource
		ncr := cpm[key]

		if ncr == nil {
			// if we don't have yet a node for closing the resource, create one and link the closing function
			// into the callback of the node.
			ncr = &nodeCloseableResource{
				Addr: rn.ResourceAddr(),
				cb:   rn.Close,
			}
			g.Add(ncr)
			cpm[key] = ncr
		}

		// Close node depends on the resource itself
		g.Connect(dag.BasicEdge(ncr, rn))

		// We need to create a dependency between the closing node and the provider of the resource that we are closing.
		// This is needed to ensure that the closing node will be added as a dependency later to the closing
		// node of the provider.
		for _, dn := range g.DownEdges(rn) {
			switch dn.(type) {
			case GraphNodeProvider:
				g.Connect(dag.BasicEdge(ncr, dn))
			}

		}

		// connect all the resource's dependencies to the close node to ensure that we are not executing the closing of the
		// resource before having all the other references satisfied
		desc, err := g.Descendents(rn)
		if err != nil {
			return err
		}
		for _, s := range desc {
			switch s.(type) {
			case GraphNodeReferencer:
				g.Connect(dag.BasicEdge(ncr, s))
			}
		}
	}
	return nil
}

// closeableResourcesVertexMap collects the vertices that are closableResource and represent an addrs.EphemeralResourceMode.
func closeableResourcesVertexMap(g *Graph) map[string]closableResource {
	m := make(map[string]closableResource)
	for _, v := range g.Vertices() {
		if n, ok := v.(closableResource); ok {
			addr := n.ResourceAddr()
			// Only ephemeral resources are closable for the moment, so ignore anything else
			if addr.Resource.Mode != addrs.EphemeralResourceMode {
				continue
			}
			m[addr.String()] = n
		}
	}
	return m
}
