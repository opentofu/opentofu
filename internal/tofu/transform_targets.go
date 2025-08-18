// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
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

// TargetingTransformer is a GraphTransformer that, when the user specifies a
// list of resources to target, or a list of resources to exclude, limits the
// graph to only those resources and their dependencies (or in the case of
// excludes - limits the graph to all resources that are not excluded or not
// dependent on excluded resources).
type TargetingTransformer struct {
	// List of targeted resource names specified by the user
	Targets []addrs.Targetable
	// List of excluded resource names specified by the user
	Excludes []addrs.Targetable
}

func (t *TargetingTransformer) Transform(_ context.Context, g *Graph) error {
	var targetedNodes dag.Set
	if len(t.Targets) > 0 {
		targetedNodes = t.selectTargetedNodes(g, t.Targets)
	} else if len(t.Excludes) > 0 {
		targetedNodes = t.removeExcludedNodes(g, t.Excludes)
	} else {
		return nil
	}

	for _, v := range g.Vertices() {
		if !targetedNodes.Include(v) {
			log.Printf("[DEBUG] Removing %q, filtered by targeting.", dag.VertexName(v))
			g.Remove(v)
		}
	}

	return nil
}

// selectTargetedNodes goes over a list of resource and modules targeted with a -target flag, and returns a set of
// targeted nodes. A targeted node is either addressed directly, address indirectly via its container, or it's a
// dependency of a targeted node.
func (t *TargetingTransformer) selectTargetedNodes(g *Graph, addrs []addrs.Targetable) dag.Set {
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

	targetedOutputNodes := t.getTargetedOutputNodes(targetedNodes, g)
	for _, outputNode := range targetedOutputNodes {
		targetedNodes.Add(outputNode)
	}

	return targetedNodes
}

func (t *TargetingTransformer) getTargetableNodeResourceAddr(v dag.Vertex) addrs.Targetable {
	switch r := v.(type) {
	case GraphNodeResourceInstance:
		return r.ResourceInstanceAddr()
	case GraphNodeConfigResource:
		return r.ResourceAddr()
	default:
		// Only resource and resource instance nodes can be targeted.
		return nil
	}
}

// removeExcludedNodes goes over a list of excluded resources and modules, and returns a set of targeted nodes to be
// used for resource targeting. An excluded resource is either addressed directly, addressed indirectly via its
// container, or it's dependent on an excluded node. The rest are the targeted nodes used for resource targeting
func (t *TargetingTransformer) removeExcludedNodes(g *Graph, excludes []addrs.Targetable) dag.Set {
	targetedNodes := make(dag.Set)
	excludedNodes := make(dag.Set)
	targetableNodes := make(dag.Set)

	vertices := g.Vertices()

	// Step 1: Find all excluded targetable nodes, and their descendants
	for _, v := range vertices {
		vertexAddr := t.getTargetableNodeResourceAddr(v)
		if vertexAddr == nil {
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
				// In general, we'd like to exclude any descendant targetable node of the current node.
				// We exclude any resource dependent on this resource (which is more general than resources dependent
				// on the resource instance, but is in-line with how -target works).
				//
				// The exception to this is when excluding a specific instance of a resource that has multiple instances.
				// During apply, the specific instance tofu.NodeApplyableResourceInstance would be dependent on the
				// resource tofu.nodeExpandApplyableResource.
				// Since we do not want to exclude all resource instances (other than the ones that we've explicitly
				// excluded), we should only exclude dependents whose target is not contained in the current node.
				depVertexAddr := t.getTargetableNodeResourceAddr(d)
				if depVertexAddr != nil && !vertexAddr.TargetContains(depVertexAddr) {
					excludedNodes.Add(d)
				}
			}
		}
	}

	// Step 2: Of the targetable nodes that were not excluded, build the graph similarly to -target
	for _, v := range targetableNodes {
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
	targetedOutputNodes := t.getTargetedOutputNodes(targetedNodes, g)
	for _, outputNode := range targetedOutputNodes {
		targetedNodes.Add(outputNode)
	}

	return targetedNodes
}

func (t *TargetingTransformer) getTargetedOutputNodes(targetedNodes dag.Set, graph *Graph) dag.Set {
	// It is expected that outputs which are only derived from targeted
	// resources are also updated. While we don't include any other possible
	// side effects from the targeted nodes, these are added because outputs
	// cannot be targeted on their own.
	//
	// Note: This behaviour has some quirks, as there are specific cases where
	// you would think an output should not be updated, but it is
	// For example, when there's a module call with an input that is dependent
	// on a root resource, and only the root resource is targeted, any output
	// that depends on a module output might be updated, if said module output
	// does not depend on any resource of the module itself.
	// Right now, we will not change this behaviour, as this has been the
	// behaviour for quite a while. A possible fix could be a more detailed
	// analysis of the outputs, and making sure that module outputs are only
	// referenced if any of the targeted nodes is in said module

	targetedOutputNodes := make(dag.Set)
	vertices := graph.Vertices()

	// Start by finding the root module output nodes themselves
	for _, v := range vertices {
		// outputs are all temporary value types
		tv, ok := v.(graphNodeTemporaryValue)
		if !ok {
			continue
		}

		// root module outputs indicate that while they are an output type,
		// they not temporary and will return false here.
		// We use walkInvalid here as we only care about the op as a workaround for nodeVariableReference, which does not apply here
		if tv.temporaryValue(walkInvalid) {
			continue
		}

		// If this output is descended only from targeted resources, then we
		// will keep it
		deps, _ := graph.Ancestors(v)
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
			targetedOutputNodes.Add(v)
			for _, d := range deps {
				targetedOutputNodes.Add(d)
			}
		}
	}

	return targetedOutputNodes
}

func (t *TargetingTransformer) nodeIsExcluded(vertexAddr addrs.Targetable, excludes []addrs.Targetable) bool {
	for _, excludeAddr := range excludes {
		if excludeAddr.TargetContains(vertexAddr) {
			return true
		}
	}

	return false
}

func (t *TargetingTransformer) nodeDescendantsExcluded(vertexAddr addrs.Targetable, excludes []addrs.Targetable) bool {
	for _, excludeAddr := range excludes {
		// The behaviour here is a bit different from targets.
		// Before expansion - We'd like to only exclude resources that were excluded by module or resource.
		//   If the excluded target is an AbsResourceInstance, then we'd want to skip exclude until we expand the resource
		// After expansion - We'd like to exclude any vertex that contains the exclude address
		//   Since before expansion the vertexAddr is without an index, then if the excludeAddr is an instance, it will
		//   only contain vertexAddr if its key is NoKey
		// So - a simple TargetContains here should be enough, both before and after expansion

		if _, ok := vertexAddr.(addrs.ConfigResource); ok {
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

func (t *TargetingTransformer) nodeIsTarget(v dag.Vertex, targets []addrs.Targetable) bool {
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
