// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"log"
	"sort"

	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/dag"
	"github.com/opentofu/opentofu/internal/lang"
)

// GraphNodeReferenceable must be implemented by any node that represents
// a OpenTofu thing that can be referenced (resource, module, etc.).
//
// Even if the thing has no name, this should return an empty list. By
// implementing this and returning a non-nil result, you say that this CAN
// be referenced and other methods of referencing may still be possible (such
// as by path!)
type GraphNodeReferenceable interface {
	GraphNodeModulePath

	// ReferenceableAddrs returns a list of addresses through which this can be
	// referenced.
	ReferenceableAddrs() []addrs.Referenceable
}

// GraphNodeReferencer must be implemented by nodes that reference other
// OpenTofu items and therefore depend on them.
type GraphNodeReferencer interface {
	GraphNodeModulePath

	// References returns a list of references made by this node, which
	// include both a referenced address and source location information for
	// the reference.
	References() []*addrs.Reference
}

// GraphNodeRootReferencer is implemented by nodes that reference the root
// module, for example module imports
type GraphNodeRootReferencer interface {
	GraphNodeReferencer

	RootReferences() []*addrs.Reference
}

type GraphNodeAttachDependencies interface {
	GraphNodeConfigResource
	AttachDependencies([]addrs.ConfigResource)
}

// graphNodeDependsOn is implemented by resources that need to expose any
// references set via DependsOn in their configuration.
type graphNodeDependsOn interface {
	GraphNodeReferencer
	DependsOn() []*addrs.Reference
}

// graphNodeAttachDataResourceDependsOn records all resources that are transitively
// referenced through depends_on in the configuration. This is used by data
// resources to determine if they can be read during the plan, or if they need
// to be further delayed until apply.
// We can only use an addrs.ConfigResource address here, because modules are
// not yet expended in the graph. While this will cause some extra data
// resources to show in the plan when their depends_on references may be in
// unrelated module instances, the fact that it only happens when there are any
// resource updates pending means we can still avoid the problem of the
// "perpetual diff"
type graphNodeAttachDataResourceDependsOn interface {
	GraphNodeConfigResource
	graphNodeDependsOn

	// AttachDataResourceDependsOn stores the discovered dependencies in the
	// resource node for evaluation later.
	//
	// The force parameter indicates that even if there are no dependencies,
	// force the data source to act as though there are for refresh purposes.
	// This is needed because yet-to-be-created resources won't be in the
	// initial refresh graph, but may still be referenced through depends_on.
	AttachDataResourceDependsOn(deps []addrs.ConfigResource, force bool)
}

// GraphNodeReferenceOutside is an interface that can optionally be implemented.
// A node that implements it can specify that its own referenceable addresses
// and/or the addresses it references are in a different module than the
// node itself.
//
// Any referenceable addresses returned by ReferenceableAddrs are interpreted
// relative to the returned selfPath.
//
// Any references returned by References are interpreted relative to the
// returned referencePath.
//
// It is valid but not required for either of these paths to match what is
// returned by method Path, though if both match the main Path then there
// is no reason to implement this method.
//
// The primary use-case for this is the nodes representing module input
// variables, since their expressions are resolved in terms of their calling
// module, but they are still referenced from their own module.
type GraphNodeReferenceOutside interface {
	// ReferenceOutside returns a path in which any references from this node
	// are resolved.
	ReferenceOutside() (selfPath, referencePath addrs.Module)
}

// ReferenceTransformer is a GraphTransformer that connects all the
// nodes that reference each other in order to form the proper ordering.
type ReferenceTransformer struct{}

func (t *ReferenceTransformer) Transform(_ context.Context, g *Graph) error {
	// Build a reference map so we can efficiently look up the references
	vs := g.Vertices()
	m := NewReferenceMap(vs)

	// Find the things that reference things and connect them
	for _, v := range vs {
		if _, ok := v.(GraphNodeDestroyer); ok {
			// destroy nodes references are not connected, since they can only
			// use their own state.
			continue
		}

		parents := m.References(v)
		parentsDbg := make([]string, len(parents))
		for i, v := range parents {
			parentsDbg[i] = dag.VertexName(v)
		}
		log.Printf(
			"[DEBUG] ReferenceTransformer: %q references: %v",
			dag.VertexName(v), parentsDbg)

		for _, parent := range parents {
			// A destroy plan relies solely on the state, so we only need to
			// ensure that temporary values are connected to get the evaluation
			// order correct. Any references to destroy nodes will cause
			// cycles, because they are connected in reverse order.
			if _, ok := parent.(GraphNodeDestroyer); ok {
				continue
			}

			if !graphNodesAreResourceInstancesInDifferentInstancesOfSameModule(v, parent) {
				g.Connect(dag.BasicEdge(v, parent))
			} else {
				log.Printf("[TRACE] ReferenceTransformer: skipping %s => %s inter-module-instance dependency", dag.VertexName(v), dag.VertexName(parent))
			}
		}

		if len(parents) > 0 {
			continue
		}
	}

	return nil
}

type depMap map[string]addrs.ConfigResource

// add stores the vertex if it represents a resource in the
// graph.
func (m depMap) add(v dag.Vertex) {
	// we're only concerned with resources which may have changes that
	// need to be applied.
	switch v := v.(type) {
	case GraphNodeResourceInstance:
		instAddr := v.ResourceInstanceAddr()
		addr := instAddr.ContainingResource().Config()
		m[addr.String()] = addr
	case GraphNodeConfigResource:
		addr := v.ResourceAddr()
		m[addr.String()] = addr
	}
}

// attachDataResourceDependsOnTransformer records all resources transitively
// referenced through a configuration depends_on.
type attachDataResourceDependsOnTransformer struct {
}

func (t attachDataResourceDependsOnTransformer) Transform(_ context.Context, g *Graph) error {
	// First we need to make a map of referenceable addresses to their vertices.
	// This is very similar to what's done in ReferenceTransformer, but we keep
	// implementation separate as they may need to change independently.
	vertices := g.Vertices()
	refMap := NewReferenceMap(vertices)

	for _, v := range vertices {
		depender, ok := v.(graphNodeAttachDataResourceDependsOn)
		if !ok {
			continue
		}

		// Only data need to attach depends_on, so they can determine if they
		// are eligible to be read during plan.
		if depender.ResourceAddr().Resource.Mode != addrs.DataResourceMode {
			continue
		}

		// depMap will only add resource references then dedupe
		deps := make(depMap)
		dependsOnDeps, fromModule := refMap.dependsOn(g, depender)
		for _, dep := range dependsOnDeps {
			// any the dependency
			deps.add(dep)
		}

		res := make([]addrs.ConfigResource, 0, len(deps))
		for _, d := range deps {
			res = append(res, d)
		}

		log.Printf("[TRACE] attachDataDependenciesTransformer: %s depends on %s", depender.ResourceAddr(), res)
		depender.AttachDataResourceDependsOn(res, fromModule)
	}

	return nil
}

// AttachDependenciesTransformer records all resource dependencies for each
// instance, and attaches the addresses to the node itself. Managed resource
// will record these in the state for proper ordering of destroy operations.
type AttachDependenciesTransformer struct {
}

func (t AttachDependenciesTransformer) Transform(_ context.Context, g *Graph) error {
	for _, v := range g.Vertices() {
		attacher, ok := v.(GraphNodeAttachDependencies)
		if !ok {
			continue
		}
		selfAddr := attacher.ResourceAddr()

		ans, err := g.Ancestors(v)
		if err != nil {
			return err
		}

		// dedupe addrs when there's multiple instances involved, or
		// multiple paths in the un-reduced graph
		depMap := map[string]addrs.ConfigResource{}
		for _, d := range ans {
			var addr addrs.ConfigResource

			switch d := d.(type) {
			case GraphNodeResourceInstance:
				instAddr := d.ResourceInstanceAddr()
				addr = instAddr.ContainingResource().Config()
			case GraphNodeConfigResource:
				addr = d.ResourceAddr()
			default:
				continue
			}

			if addr.Equal(selfAddr) {
				continue
			}
			depMap[addr.String()] = addr
		}

		deps := make([]addrs.ConfigResource, 0, len(depMap))
		for _, d := range depMap {
			deps = append(deps, d)
		}
		sort.Slice(deps, func(i, j int) bool {
			return deps[i].String() < deps[j].String()
		})

		log.Printf("[TRACE] AttachDependenciesTransformer: %s depends on %s", attacher.ResourceAddr(), deps)
		attacher.AttachDependencies(deps)
	}

	return nil
}

func isDependableResource(v dag.Vertex) bool {
	switch v.(type) {
	case GraphNodeResourceInstance:
		return true
	case GraphNodeConfigResource:
		return true
	}
	return false
}

// ReferenceMap is a structure that can be used to efficiently check
// for references on a graph, mapping internal reference keys (as produced by
// the mapKey method) to one or more vertices that are identified by each key.
type ReferenceMap map[string][]dag.Vertex

// References returns the set of vertices that the given vertex refers to,
// and any referenced addresses that do not have corresponding vertices.
func (m ReferenceMap) References(v dag.Vertex) []dag.Vertex {
	rn, ok := v.(GraphNodeReferencer)
	if !ok {
		return nil
	}

	var matches []dag.Vertex

	if rrn, ok := rn.(GraphNodeRootReferencer); ok {
		for _, ref := range rrn.RootReferences() {
			matches = append(matches, m.addReference(addrs.RootModule, v, ref)...)
		}
	}

	for _, ref := range rn.References() {
		matches = append(matches, m.addReference(vertexReferencePath(v), v, ref)...)
	}

	return matches
}

// addReference returns the set of vertices that the given reference requires
// within a given module.  It additionally excludes the current vertex.
func (m ReferenceMap) addReference(path addrs.Module, current dag.Vertex, ref *addrs.Reference) []dag.Vertex {
	var matches []dag.Vertex

	subject := ref.Subject

	key := m.mapKey(path, subject)
	if _, exists := m[key]; !exists {
		// If what we were looking for was a ResourceInstance then we
		// might be in a resource-oriented graph rather than an
		// instance-oriented graph, and so we'll see if we have the
		// resource itself instead.
		switch ri := subject.(type) {
		case addrs.ResourceInstance:
			subject = ri.ContainingResource()
		case addrs.ResourceInstancePhase:
			subject = ri.ContainingResource()
		case addrs.ModuleCallInstanceOutput:
			subject = ri.ModuleCallOutput()
		case addrs.ModuleCallInstance:
			subject = ri.Call
		case addrs.ProviderFunction:
			return nil
		default:
			log.Printf("[INFO] ReferenceTransformer: reference not found: %q", subject)
			return nil
		}
		key = m.mapKey(path, subject)
	}
	vertices := m[key]
	for _, rv := range vertices {
		// don't include self-references
		if rv == current {
			continue
		}
		matches = append(matches, rv)
	}
	return matches
}

// dependsOn returns the set of vertices that the given vertex refers to from
// the configured depends_on. The bool return value indicates if depends_on was
// found in a parent module configuration.
func (m ReferenceMap) dependsOn(g *Graph, depender graphNodeDependsOn) ([]dag.Vertex, bool) {
	var res []dag.Vertex
	fromModule := false

	refs := depender.DependsOn()

	// get any implied dependencies for data sources
	refs = append(refs, m.dataDependsOn(depender)...)

	// This is where we record that a module has depends_on configured.
	if _, ok := depender.(*nodeExpandModule); ok && len(refs) > 0 {
		fromModule = true
	}

	for _, ref := range refs {
		subject := ref.Subject

		key := m.referenceMapKey(depender, subject)
		vertices, ok := m[key]
		if !ok {
			// the ReferenceMap generates all possible keys, so any warning
			// here is probably not useful for this implementation.
			continue
		}
		for _, rv := range vertices {
			// don't include self-references
			if rv == depender {
				continue
			}
			res = append(res, rv)

			// Check any ancestors for transitive dependencies when we're
			// not pointed directly at a resource. We can't be much more
			// precise here, since in order to maintain our guarantee that data
			// sources will wait for explicit dependencies, if those dependencies
			// happen to be a module, output, or variable, we have to find some
			// upstream managed resource in order to check for a planned
			// change.
			if _, ok := rv.(GraphNodeConfigResource); !ok {
				ans, _ := g.Ancestors(rv)
				for _, v := range ans {
					if isDependableResource(v) {
						res = append(res, v)
					}
				}
			}
		}
	}

	parentDeps, fromParentModule := m.parentModuleDependsOn(g, depender)
	res = append(res, parentDeps...)

	return res, fromModule || fromParentModule
}

// Return extra depends_on references if this is a data source.
// For data sources we implicitly treat references to managed resources as
// depends_on entries. If a data source references a managed resource, even if
// that reference is resolvable, it stands to reason that the user intends for
// the data source to require that resource in some way.
func (m ReferenceMap) dataDependsOn(depender graphNodeDependsOn) []*addrs.Reference {
	var refs []*addrs.Reference
	if n, ok := depender.(GraphNodeConfigResource); ok &&
		n.ResourceAddr().Resource.Mode == addrs.DataResourceMode {
		for _, r := range depender.References() {

			var resAddr addrs.Resource
			switch s := r.Subject.(type) {
			case addrs.Resource:
				resAddr = s
			case addrs.ResourceInstance:
				resAddr = s.Resource
				r.Subject = resAddr
			case addrs.ProviderFunction:
				continue
			}

			if resAddr.Mode != addrs.ManagedResourceMode {
				// We only want to wait on directly referenced managed resources.
				// Data sources have no external side effects, so normal
				// references to them in the config will suffice for proper
				// ordering.
				continue
			}

			refs = append(refs, r)
		}
	}
	return refs
}

// parentModuleDependsOn returns the set of vertices that a data sources parent
// module references through the module call's depends_on. The bool return
// value indicates if depends_on was found in a parent module configuration.
func (m ReferenceMap) parentModuleDependsOn(g *Graph, depender graphNodeDependsOn) ([]dag.Vertex, bool) {
	var res []dag.Vertex
	fromModule := false

	// Look for containing modules with DependsOn.
	// This should be connected directly to the module node, so we only need to
	// look one step away.
	for _, v := range g.DownEdges(depender) {
		// we're only concerned with module expansion nodes here.
		mod, ok := v.(*nodeExpandModule)
		if !ok {
			continue
		}

		deps, fromParentModule := m.dependsOn(g, mod)
		for _, dep := range deps {
			// add the dependency
			res = append(res, dep)

			// and check any transitive resource dependencies for more resources
			ans, _ := g.Ancestors(dep)
			for _, v := range ans {
				if isDependableResource(v) {
					res = append(res, v)
				}
			}
		}
		fromModule = fromModule || fromParentModule
	}

	return res, fromModule
}

func (m *ReferenceMap) mapKey(path addrs.Module, addr addrs.Referenceable) string {
	return fmt.Sprintf("%s|%s", path.String(), addr.String())
}

// vertexReferenceablePath returns the path in which the given vertex can be
// referenced. This is the path that its results from ReferenceableAddrs
// are considered to be relative to.
//
// Only GraphNodeModulePath implementations can be referenced, so this method will
// panic if the given vertex does not implement that interface.
func vertexReferenceablePath(v dag.Vertex) addrs.Module {
	sp, ok := v.(GraphNodeModulePath)
	if !ok {
		// Only nodes with paths can participate in a reference map.
		panic(fmt.Errorf("vertexMapKey on vertex type %T which doesn't implement GraphNodeModulePath", sp))
	}

	if outside, ok := v.(GraphNodeReferenceOutside); ok {
		// Vertex is referenced from a different module than where it was
		// declared.
		path, _ := outside.ReferenceOutside()
		return path
	}

	// Vertex is referenced from the same module as where it was declared.
	return sp.ModulePath()
}

// vertexReferencePath returns the path in which references _from_ the given
// vertex must be interpreted.
//
// Only GraphNodeModulePath implementations can have references, so this method
// will panic if the given vertex does not implement that interface.
func vertexReferencePath(v dag.Vertex) addrs.Module {
	sp, ok := v.(GraphNodeModulePath)
	if !ok {
		// Only nodes with paths can participate in a reference map.
		panic(fmt.Errorf("vertexReferencePath on vertex type %T which doesn't implement GraphNodeModulePath", v))
	}

	if outside, ok := v.(GraphNodeReferenceOutside); ok {
		// Vertex makes references to objects in a different module than where
		// it was declared.
		_, path := outside.ReferenceOutside()
		return path
	}

	// Vertex makes references to objects in the same module as where it
	// was declared.
	return sp.ModulePath()
}

// referenceMapKey produces keys for the "edges" map. "referrer" is the vertex
// that the reference is from, and "addr" is the address of the object being
// referenced.
//
// The result is an opaque string that includes both the address of the given
// object and the address of the module instance that object belongs to.
//
// Only GraphNodeModulePath implementations can be referrers, so this method will
// panic if the given vertex does not implement that interface.
func (m *ReferenceMap) referenceMapKey(referrer dag.Vertex, addr addrs.Referenceable) string {
	path := vertexReferencePath(referrer)
	return m.mapKey(path, addr)
}

// NewReferenceMap is used to create a new reference map for the
// given set of vertices.
func NewReferenceMap(vs []dag.Vertex) ReferenceMap {
	// Build the lookup table
	m := make(ReferenceMap)
	for _, v := range vs {
		// We're only looking for referenceable nodes
		rn, ok := v.(GraphNodeReferenceable)
		if !ok {
			continue
		}

		path := vertexReferenceablePath(v)

		// Go through and cache them
		for _, addr := range rn.ReferenceableAddrs() {
			key := m.mapKey(path, addr)
			m[key] = append(m[key], v)
		}
	}

	return m
}

// ReferencesFromConfig returns the references that a configuration has
// based on the interpolated variables in a configuration.
func ReferencesFromConfig(body hcl.Body, schema *configschema.Block) []*addrs.Reference {
	if body == nil {
		return nil
	}
	refs, _ := lang.ReferencesInBlock(addrs.ParseRef, body, schema)
	return refs
}
