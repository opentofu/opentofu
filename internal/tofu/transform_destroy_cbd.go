// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"log"

	"github.com/opentofu/opentofu/internal/dag"
)

// GraphNodeDestroyerCBD must be implemented by nodes that might be
// create-before-destroy destroyers, or might plan a create-before-destroy
// action.
type GraphNodeDestroyerCBD interface {
	// CreateBeforeDestroy returns true if this node represents a node
	// that is doing a CBD.
	CreateBeforeDestroy() bool

	// ModifyCreateBeforeDestroy is called when the CBD state of a node
	// is changed dynamically. This can return an error if this isn't
	// allowed.
	ModifyCreateBeforeDestroy(bool) error
}

// ForcedCBDTransformer detects when a particular CBD-able graph node has
// dependencies with another that has create_before_destroy set that require
// it to be forced on, and forces it on.
//
// This must be used in the plan graph builder to ensure that
// create_before_destroy settings are properly propagated before constructing
// the planned changes. This requires that the plannable resource nodes
// implement GraphNodeDestroyerCBD.
type ForcedCBDTransformer struct {
}

func (t *ForcedCBDTransformer) Transform(g *Graph) error {
	for _, v := range g.Vertices() {
		dn, ok := v.(GraphNodeDestroyerCBD)
		if !ok {
			continue
		}

		if !dn.CreateBeforeDestroy() {
			// If there are no CBD descendent (dependent nodes), then we
			// do nothing here.
			if !t.hasCBDDescendent(g, v) {
				log.Printf("[TRACE] ForcedCBDTransformer: %q (%T) has no CBD descendent, so skipping", dag.VertexName(v), v)
				continue
			}

			// If this isn't naturally a CBD node, this means that an descendent is
			// and we need to auto-upgrade this node to CBD. We do this because
			// a CBD node depending on non-CBD will result in cycles. To avoid this,
			// we always attempt to upgrade it.
			log.Printf("[TRACE] ForcedCBDTransformer: forcing create_before_destroy on for %q (%T)", dag.VertexName(v), v)
			if err := dn.ModifyCreateBeforeDestroy(true); err != nil {
				return fmt.Errorf(
					"%s: must have create before destroy enabled because "+
						"a dependent resource has CBD enabled. However, when "+
						"attempting to automatically do this, an error occurred: %w",
					dag.VertexName(v), err)
			}
		} else {
			log.Printf("[TRACE] ForcedCBDTransformer: %q (%T) already has create_before_destroy set", dag.VertexName(v), v)
		}
	}
	return nil
}

// hasCBDDescendent returns true if any descendent (node that depends on this)
// has CBD set.
func (t *ForcedCBDTransformer) hasCBDDescendent(g *Graph, v dag.Vertex) bool {
	s, _ := g.Descendents(v)
	if s == nil {
		return true
	}

	for _, ov := range s {
		dn, ok := ov.(GraphNodeDestroyerCBD)
		if !ok {
			continue
		}

		if dn.CreateBeforeDestroy() {
			// some descendent is CreateBeforeDestroy, so we need to follow suit
			log.Printf("[TRACE] ForcedCBDTransformer: %q has CBD descendent %q", dag.VertexName(v), dag.VertexName(ov))
			return true
		}
	}

	return false
}

// CBDEdgeTransformer modifies the edges of create-before-destroy ("CBD") nodes
// that went through the DestroyEdgeTransformer so that they will have the
// correct dependencies. There are two parts to this:
//
//  1. With CBD, the destroy edge is inverted: the destroy depends on
//     the creation.
//
//  2. Destroy for A must depend on resources that depend on A. This is to
//     allow the destroy to only happen once nodes that depend on A successfully
//     update to A. Example: adding a web server updates the load balancer
//     before deleting the old web server.
//
// This transformer requires that a previous transformer has already forced
// create_before_destroy on for nodes that are depended on by explicit CBD
// nodes. This is the logic in ForcedCBDTransformer, though in practice we
// will get here by recording the CBD-ness of each change in the plan during
// the plan walk and then forcing the nodes into the appropriate setting during
// DiffTransformer when building the apply graph.
type CBDEdgeTransformer struct{}

func (t *CBDEdgeTransformer) Transform(g *Graph) error {
	// Go through and reverse any destroy edges
	for _, v := range g.Vertices() {
		dn, ok := v.(GraphNodeDestroyerCBD)
		if !ok {
			continue
		}
		if _, ok = v.(GraphNodeDestroyer); !ok {
			continue
		}

		if !dn.CreateBeforeDestroy() {
			continue
		}

		// Find the resource edges
		for _, e := range g.EdgesTo(v) {
			src := e.Source()

			// If source is a create node, invert the edge.
			// This covers both the node's own creator, as well as reversing
			// any dependants' edges.
			if _, ok := src.(GraphNodeCreator); ok {
				log.Printf("[TRACE] CBDEdgeTransformer: reversing edge %s -> %s", dag.VertexName(src), dag.VertexName(v))
				g.RemoveEdge(e)
				g.Connect(dag.BasicEdge(v, src))
			}
		}
	}
	return nil
}
