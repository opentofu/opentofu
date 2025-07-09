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

// resourceCloser is the definition of the function that needs to exist on the node that wants to release external
// resources before exit of the OpenTofu process.
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
	closeableVertices := closeableResourcesVertexMap(g)
	cpm := make(map[string]struct{})

	for key, instances := range closeableVertices {
		// check if we already generated a closing node for that particular resource
		_, exists := cpm[key]
		if exists {
			continue
		}

		// gather the references for the closing function from all existing instances of the resource
		var callbacks []resourceCloser
		var addr addrs.ConfigResource
		var allDeps []dag.Vertex
		for _, inst := range instances {
			deps, err := t.collectInstanceDependencies(g, inst)
			if err != nil {
				return err
			}
			// collect all vertices inst is linked to
			allDeps = append(allDeps, deps...)
			// and collect inst as a dependency too, later it will be linked to the node responsible with closing it
			allDeps = append(allDeps, inst)
			callbacks = append(callbacks, inst.Close)
			// Should be the same for all the instances since closeableResourcesVertexMap is generating the key
			// based on each vertex addr. Therefore, no issue here on overwriting it on each iteration.
			addr = inst.ResourceAddr()
		}
		// we postponed creation of the node and its addition to the graph, just to ensure that we are having all the
		// required information prepared without errors before adding this node into the graph.
		ncr := &nodeCloseableResource{
			Addr: addr,
			cbs:  callbacks,
		}
		g.Add(ncr)
		for _, dep := range allDeps {
			g.Connect(dag.BasicEdge(ncr, dep))
		}
		cpm[key] = struct{}{}
	}
	return nil
}

// collectInstanceDependencies gathers all dependencies of the given node for being linked to the closing node of inst.
//
// To do so, we need to gather two types of dependencies:
//   - we need to gather all the provider node of inst. This is needed to ensure that the resource closing
//     node will be added as a dependency later to the closing node of the provider, to force the right
//     order of nodes execution.
//   - gather all the nodes dependent on inst to ensure that we are not executing the closing of the
//     resource before having all the other references satisfied.
func (t *CloseableResourceTransformer) collectInstanceDependencies(g *Graph, inst closableResource) ([]dag.Vertex, error) {
	var deps []dag.Vertex

	for _, dn := range g.DownEdges(inst) {
		switch dn.(type) {
		case GraphNodeProvider:
			deps = append(deps, dn)
		}
	}

	desc, err := g.Descendents(inst)
	if err != nil {
		return nil, err
	}
	for _, s := range desc {
		switch s.(type) {
		case GraphNodeReferencer:
			deps = append(deps, s)
		}
	}

	return deps, nil
}

// closeableResourcesVertexMap collects the vertices that are closableResource and represent an addrs.EphemeralResourceMode.
func closeableResourcesVertexMap(g *Graph) map[string][]closableResource {
	m := make(map[string][]closableResource)
	for _, v := range g.Vertices() {
		if n, ok := v.(closableResource); ok {
			addr := n.ResourceAddr()
			// Only ephemeral resources are closable for the moment, so ignore anything else
			if addr.Resource.Mode != addrs.EphemeralResourceMode {
				continue
			}
			l := m[addr.String()]
			m[addr.String()] = append(l, n)
		}
	}
	return m
}
