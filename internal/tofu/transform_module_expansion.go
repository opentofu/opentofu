// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/dag"
)

// ModuleExpansionTransformer is a GraphTransformer that adds graph nodes
// representing the possible expansion of each module call in the configuration,
// and ensures that any nodes representing objects declared within a module
// are dependent on the expansion node so that they will be visited only
// after the module expansion has been decided.
//
// This transform must be applied only after all nodes representing objects
// that can be contained within modules have already been added.
type ModuleExpansionTransformer struct {
	Config *configs.Config

	// Concrete allows injection of a wrapped module node by the graph builder
	// to alter the evaluation behavior.
	Concrete ConcreteModuleNodeFunc

	closers map[string]*nodeCloseModule
}

func (t *ModuleExpansionTransformer) Transform(_ context.Context, g *Graph) error {
	t.closers = make(map[string]*nodeCloseModule)

	// Construct a tree for fast lookups of Vertices based on their ModulePath.
	tree := &pathTree{
		children: make(map[string]*pathTree),
	}

	for _, v := range g.Vertices() {
		tree.addVertex(v)
	}

	// The root module is always a singleton and so does not need expansion
	// processing, but any descendent modules do. We'll process them
	// recursively using t.transform.
	for _, cfg := range t.Config.Children {
		err := t.transform(g, cfg, tree, nil)
		if err != nil {
			return err
		}
	}

	// Now go through and connect all nodes to their respective module closers.
	// This is done all at once here, because orphaned modules were already
	// handled by the RemovedModuleTransformer, and those module closers are in
	// the graph already, and need to be connected to their parent closers.
	for _, v := range g.Vertices() {
		switch v.(type) {
		case GraphNodeDestroyer:
			// Destroy nodes can only be ordered relative to other resource
			// instances.
			continue
		case *nodeCloseModule:
			// a module closer cannot connect to itself
			continue
		case *nodeExpandCheck, *nodeReportCheck, *nodeCheckAssert:
			// Check-related nodes are not module-close dependencies because
			// they don't produce any values that could potentially contribute
			// to a module's output values, and skipping these edges avoids
			// dependency cycles when a module containing checks is used
			// in a depends_on in the parent module.
			continue
		}

		// Also skip data sources nested inside check blocks for the same reason as above.
		if t.isNestedCheckDataSource(v) {
			continue
		}

		// any node that executes within the scope of a module should be a
		// GraphNodeModulePath
		pather, ok := v.(GraphNodeModulePath)
		if !ok {
			continue
		}
		if closer, ok := t.closers[pather.ModulePath().String()]; ok {
			// The module closer depends on each child resource instance, since
			// during apply the module expansion will complete before the
			// individual instances are applied.
			g.Connect(dag.BasicEdge(closer, v))
		}
	}

	// Modules implicitly depend on their child modules, so connect closers to
	// other which contain their path.
	for _, c := range t.closers {
		// For a closer c with address ["module.foo", "module.bar", "module.baz"],
		// we'll look up all potential parent modules:
		//
		// - t.closers["module.foo"]
		// - t.closers["module.foo.module.bar"]
		//
		// And connect the parent module to c.
		//
		// We skip i=0 because c.Addr[0:0] == [], and the root module should not exist in t.closers.
		for i := 1; i < len(c.Addr); i++ {
			parentAddr := c.Addr[0:i].String()
			if parent, ok := t.closers[parentAddr]; ok {
				g.Connect(dag.BasicEdge(parent, c))
			}
		}
	}

	return nil
}

func (t *ModuleExpansionTransformer) transform(g *Graph, c *configs.Config, tree *pathTree, parentNode dag.Vertex) error {
	_, call := c.Path.Call()
	modCall := c.Parent.Module.ModuleCalls[call.Name]

	n := &nodeExpandModule{
		Addr:       c.Path,
		Config:     c.Module,
		ModuleCall: modCall,
	}
	var expander dag.Vertex = n
	if t.Concrete != nil {
		expander = t.Concrete(n)
	}

	g.Add(expander)
	tree.addVertex(expander)
	log.Printf("[TRACE] ModuleExpansionTransformer: Added %s as %T", c.Path, expander)

	if parentNode != nil {
		log.Printf("[TRACE] ModuleExpansionTransformer: %s must wait for expansion of %s", dag.VertexName(expander), dag.VertexName(parentNode))
		g.Connect(dag.BasicEdge(expander, parentNode))
	}

	// Add the closer (which acts as the root module node) to provide a
	// single exit point for the expanded module.
	closer := &nodeCloseModule{
		Addr: c.Path,
	}
	g.Add(closer)
	tree.addVertex(closer)
	g.Connect(dag.BasicEdge(closer, expander))
	t.closers[c.Path.String()] = closer

	for _, childV := range tree.findModule(c.Path) {
		// don't connect a node to itself
		if childV == expander {
			continue
		}

		var path addrs.Module
		switch t := childV.(type) {
		case GraphNodeDestroyer:
			// skip destroyers, as they can only depend on other resources.
			continue

		case GraphNodeModulePath:
			path = t.ModulePath()
		default:
			continue
		}

		if path.Equal(c.Path) {
			log.Printf("[TRACE] ModuleExpansionTransformer: %s must wait for expansion of %s", dag.VertexName(childV), c.Path)
			g.Connect(dag.BasicEdge(childV, expander))
		}
	}

	// Also visit child modules, recursively.
	for _, cc := range c.Children {
		if err := t.transform(g, cc, tree, expander); err != nil {
			return err
		}
	}

	return nil
}

// pathTree is a tree containing a dag.Set of dag.Vertex per addrs.Module
//
// Given V = vertices in the graph and M = modules in the graph, constructing
// the tree takes ~O(V*log(M)) time to insert all the vertices, which gives
// us ~O(log(M)) access time to find all vertices that are part of a module.
//
// The previous implementation iterated over every node for each module, which made
// Transform() take O(V * M).
//
// This improves that to O(V*log(M) + M).
type pathTree struct {
	children map[string]*pathTree
	leaves   dag.Set
}

func (t *pathTree) addVertex(v dag.Vertex) {
	mp, ok := v.(GraphNodeModulePath)
	if !ok {
		return
	}

	t.add(v, mp.ModulePath())
}

func (t *pathTree) add(v dag.Vertex, addr []string) {
	if len(addr) == 0 {
		if t.leaves == nil {
			t.leaves = make(dag.Set)
		}
		t.leaves.Add(v)
		return
	}

	next, addr := addr[0], addr[1:]
	child, ok := t.children[next]
	if !ok {
		child = &pathTree{
			children: make(map[string]*pathTree),
		}
		t.children[next] = child
	}

	child.add(v, addr)
}

func (t *pathTree) findModule(p addrs.Module) dag.Set {
	return t.find(p)
}

func (t *pathTree) find(addr []string) dag.Set {
	if len(addr) == 0 {
		return t.leaves
	}

	next, addr := addr[0], addr[1:]
	child, ok := t.children[next]
	if !ok {
		return nil
	}

	return child.find(addr)
}

// isNestedCheckDataSource returns true if the vertex represents a data source
// that is nested inside a check block. Such data sources are observational
// and should not participate in module dependency ordering.
func (t *ModuleExpansionTransformer) isNestedCheckDataSource(v dag.Vertex) bool {
	cfgNode, ok := v.(GraphNodeConfigResource)
	if !ok {
		return false
	}

	addr := cfgNode.ResourceAddr()
	if addr.Resource.Mode != addrs.DataResourceMode {
		return false
	}

	config := t.Config.Descendent(addr.Module)
	if config == nil {
		return false
	}

	resource := config.Module.DataResources[addr.Resource.String()]
	if resource != nil && resource.Container != nil {
		_, isCheck := resource.Container.(*configs.Check)
		return isCheck
	}

	return false
}
