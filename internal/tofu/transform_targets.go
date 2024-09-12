// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/dag"
)

// GraphNodeTargetable is an interface for graph nodes to implement when they
// need to be told about incoming targets or excluded targets. This is useful for
// nodes that need to respect targets and excludes as they dynamically expand.
// Note that the lists of targets and excludes provided will contain every target
// or every exclude provided, and each implementing graph node must filter this
// list to targets considered relevant.
type GraphNodeTargetable interface {
	SetTargets([]addrs.Targetable)
	SetExcludes([]addrs.Targetable)
}

// TargetsTransformer is a GraphTransformer that, when the user specifies a
// list of resources to target, or a list of resources to exclude, limits the
// graph to only those resources and their dependencies (or in the case of
// excludes - limits the graph to all resources that are not excluded or not
// dependent on excluded resources).
type TargetsTransformer struct {
	// List of targeted resource names specified by the user
	Targets []addrs.Targetable
	// List of excluded resource names specified by the user
	Excludes []addrs.Targetable
}

func (t *TargetsTransformer) Transform(g *Graph) error {
	var targetedNodes dag.Set
	var err error
	if len(t.Targets) > 0 {
		targetedNodes, err = t.selectTargetedNodes(g, t.Targets)
	} else if len(t.Excludes) > 0 {
		targetedNodes, err = t.removeExcludedNodes(g, t.Excludes)
	} else {
		return nil
	}

	if err != nil {
		return err
	}

	for _, v := range g.Vertices() {
		if !targetedNodes.Include(v) {
			log.Printf("[DEBUG] Removing %q, filtered by targeting.", dag.VertexName(v))
			g.Remove(v)
		}
	}

	return nil
}

// Returns a set of targeted nodes. A targeted node is either addressed
// directly, address indirectly via its container, or it's a dependency of a
// targeted node.
func (t *TargetsTransformer) selectTargetedNodes(g *Graph, addrs []addrs.Targetable) (dag.Set, error) {
	targetedNodes := make(dag.Set)

	vertices := g.Vertices()

	for _, v := range vertices {
		if t.nodeIsTarget(v, addrs) {
			targetedNodes.Add(v)

			// We inform nodes that ask about the list of targets - helps for nodes
			// that need to dynamically expand. Note that this only occurs for nodes
			// that are already directly targeted.
			if tn, ok := v.(GraphNodeTargetable); ok {
				tn.SetTargets(addrs)
			}

			deps, _ := g.Ancestors(v)
			for _, d := range deps {
				targetedNodes.Add(d)
			}
		}
	}

	// It is expected that outputs which are only derived from targeted
	// resources are also updated. While we don't include any other possible
	// side effects from the targeted nodes, these are added because outputs
	// cannot be targeted on their own.
	// Start by finding the root module output nodes themselves
	for _, v := range vertices {
		// outputs are all temporary value types
		tv, ok := v.(graphNodeTemporaryValue)
		if !ok {
			continue
		}

		// root module outputs indicate that while they are an output type,
		// they not temporary and will return false here.
		if tv.temporaryValue() {
			continue
		}

		// If this output is descended only from targeted resources, then we
		// will keep it
		deps, _ := g.Ancestors(v)
		found := 0
		for _, d := range deps {
			switch d.(type) {
			case GraphNodeResourceInstance:
			case GraphNodeConfigResource:
			default:
				continue
			}

			if !targetedNodes.Include(d) {
				// this dependency isn't being targeted, so we can't process this
				// output
				found = 0
				break
			}

			found++
		}

		if found > 0 {
			// we found an output we can keep; add it, and all it's dependencies
			targetedNodes.Add(v)
			for _, d := range deps {
				targetedNodes.Add(d)
			}
		}
	}

	return targetedNodes, nil
}

// Returns a set of targeted nodes. A targeted node is either addressed
// directly, address indirectly via its container, or it's a dependency of a
// targeted node.
func (t *TargetsTransformer) removeExcludedNodes(g *Graph, excludes []addrs.Targetable) (dag.Set, error) {
	targetedNodes := make(dag.Set)
	excludedNodes := make(dag.Set)
	targetableNodes := make(dag.Set)

	vertices := g.Vertices()

	// Step 1: Find all excluded targetable nodes, and their descendants
	for _, v := range vertices {
		var vertexAddr addrs.Targetable
		switch r := v.(type) {
		case GraphNodeResourceInstance:
			vertexAddr = r.ResourceInstanceAddr()
		case GraphNodeConfigResource:
			vertexAddr = r.ResourceAddr()
		default:
			// Only resource and resource instance nodes can be targeted.
			continue
		}
		targetableNodes.Add(v)

		nodeExcluded := t.nodeIsExcluded(vertexAddr, excludes)
		if nodeExcluded {
			excludedNodes.Add(v)
		}

		if nodeExcluded || t.nodeDescendantsExcluded(vertexAddr, excludes) {
			deps, _ := g.Descendents(v)
			for _, d := range deps {
				excludedNodes.Add(d)
			}
		}
	}

	// Step 2: Of the targetable nodes that were not excluded, build the graph similarly to -target
	for _, v := range vertices {
		if !excludedNodes.Include(v) {
			targetedNodes.Add(v)

			// We inform nodes that ask about the list of excludes - helps for nodes
			// that need to dynamically expand. Note that this only occurs for nodes
			// that are targetable and we didn't exclude
			if tn, ok := v.(GraphNodeTargetable); ok {
				tn.SetExcludes(excludes)
			}

			deps, _ := g.Ancestors(v)
			for _, d := range deps {
				targetedNodes.Add(d)
			}
		}
	}

	// Step 3: Add outputs
	// It is expected that outputs which are only derived from targeted
	// resources are also updated. While we don't include any other possible
	// side effects from the targeted nodes, these are added because outputs
	// cannot be targeted on their own.
	// Start by finding the root module output nodes themselves
	for _, v := range vertices {
		// outputs are all temporary value types
		tv, ok := v.(graphNodeTemporaryValue)
		if !ok {
			continue
		}

		// root module outputs indicate that while they are an output type,
		// they not temporary and will return false here.
		if tv.temporaryValue() {
			continue
		}

		// If this output is descended only from targeted resources, then we
		// will keep it
		deps, _ := g.Ancestors(v)
		found := 0
		for _, d := range deps {
			switch d.(type) {
			case GraphNodeResourceInstance:
			case GraphNodeConfigResource:
			default:
				continue
			}

			if excludedNodes.Include(d) {
				// this dependency is excluded, so we can't process this
				// output
				found = 0
				break
			}

			found++
		}

		if found > 0 {
			// we found an output we can keep; add it, and all it's dependencies
			targetedNodes.Add(v)
			for _, d := range deps {
				targetedNodes.Add(d)
			}
		}
	}

	return targetedNodes, nil
}

func (t *TargetsTransformer) nodeIsExcluded(vertexAddr addrs.Targetable, excludes []addrs.Targetable) bool {
	for _, excludeAddr := range excludes {
		// The behaviour here is a bit different from targets.
		// Before expansion - We'd like to only exclude resources that were excluded by module or resource.
		//   If the excluded target is an AbsResourceInstance, then we'd want to skip exclude until we expand the resource
		// After expansion - We'd like to exclude any vertex that contains the exclude address
		//   Since before expansion the vertexAddr is without an index, then if the excludeAddr is an instance, it will
		//   only contain vertexAddr if its key is NoKey
		// So - a simple TargetContains here should be enough, both before and after expansion

		if excludeAddr.TargetContains(vertexAddr) {
			return true
		}
	}

	return false
}

func (t *TargetsTransformer) nodeDescendantsExcluded(vertexAddr addrs.Targetable, excludes []addrs.Targetable) bool {
	for _, excludeAddr := range excludes {
		// The behaviour here is a bit different from targets.
		// Before expansion - We'd like to only exclude resources that were excluded by module or resource.
		//   If the excluded target is an AbsResourceInstance, then we'd want to skip exclude until we expand the resource
		// After expansion - We'd like to exclude any vertex that contains the exclude address
		//   Since before expansion the vertexAddr is without an index, then if the excludeAddr is an instance, it will
		//   only contain vertexAddr if its key is NoKey
		// So - a simple TargetContains here should be enough, both before and after expansion

		switch vertexAddr.(type) {
		case addrs.ConfigResource:
			// Before expansion happens, we only have nodes that know their
			// ConfigResource address.  We need to take the more specific
			// target addresses and generalize them in order to compare with a
			// ConfigResource.
			//
			// If the excluded target, in is generalized form, contains the vertex address, then we know that we could remove the descendants
			// even if we don't remove the node itself from the graph. However, this could cause cases where too many resources are excluded.
			// For example, with -exclude=null_resource.a[1], and a null_resource.b[*] for which each instance depends on a single null_resource.a instance,
			// all null_resource.b instances will be excluded. This is not accurate, but is in line with -target today, which over-targets dependencies
			switch target := excludeAddr.(type) {
			case addrs.AbsResourceInstance:
				excludeAddr = target.ContainingResource().Config()
			case addrs.AbsResource:
				excludeAddr = target.Config()
			case addrs.ModuleInstance:
				excludeAddr = target.Module()
			}
		}

		if excludeAddr.TargetContains(vertexAddr) {
			return true
		}
	}

	return false
}

func (t *TargetsTransformer) nodeIsTarget(v dag.Vertex, targets []addrs.Targetable) bool {
	var vertexAddr addrs.Targetable
	switch r := v.(type) {
	case GraphNodeResourceInstance:
		vertexAddr = r.ResourceInstanceAddr()
	case GraphNodeConfigResource:
		vertexAddr = r.ResourceAddr()

	default:
		// Only resource and resource instance nodes can be targeted.
		return false
	}

	for _, targetAddr := range targets {
		switch vertexAddr.(type) {
		case addrs.ConfigResource:
			// Before expansion happens, we only have nodes that know their
			// ConfigResource address.  We need to take the more specific
			// target addresses and generalize them in order to compare with a
			// ConfigResource.
			switch target := targetAddr.(type) {
			case addrs.AbsResourceInstance:
				targetAddr = target.ContainingResource().Config()
			case addrs.AbsResource:
				targetAddr = target.Config()
			case addrs.ModuleInstance:
				targetAddr = target.Module()
			}
		}

		if targetAddr.TargetContains(vertexAddr) {
			return true
		}
	}

	return false
}
