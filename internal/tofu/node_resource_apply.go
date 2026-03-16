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
	"github.com/opentofu/opentofu/internal/dag"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// nodeExpandApplyableResource handles the first layer of resource
// expansion during apply. Even though the resource instances themselves are already expanded from the plan,
// we still need to register resource nodes in instances.Expander with their absolute addresses
// based on the expanded module instances.
//
// "Expand" in the name refers to instances.Expander usage. This node doesn't generate a dynamic subgraph.
type nodeExpandApplyableResource struct {
	*NodeAbstractResource

	// This slice is meant to keep references to the resourceCloser's of the expanded instances.
	// Later, this will be called from nodeCloseableResource.
	// At the time of introducing this, it was strictly meant for ephemeral resources, but if there
	// will be other closeable resources, this could be used for those too.
	closers []resourceCloser
}

var (
	_ GraphNodeExecutable           = (*nodeExpandApplyableResource)(nil)
	_ GraphNodeDynamicExpandable    = (*nodeExpandApplyableResource)(nil)
	_ GraphNodeReferenceable        = (*nodeExpandApplyableResource)(nil)
	_ GraphNodeReferencer           = (*nodeExpandApplyableResource)(nil)
	_ GraphNodeConfigResource       = (*nodeExpandApplyableResource)(nil)
	_ GraphNodeAttachResourceConfig = (*nodeExpandApplyableResource)(nil)
	// nodeExpandApplyableResource needs to be retained during unused nodes pruning
	// to register the resource for expanded module instances in `instances.Expander`
	_ graphNodeRetainedByPruneUnusedNodesTransformer = (*nodeExpandApplyableResource)(nil)
	_ GraphNodeTargetable                            = (*nodeExpandApplyableResource)(nil)
	_ closableResource                               = (*nodeExpandApplyableResource)(nil)
)

func (n *nodeExpandApplyableResource) retainDuringUnusedPruning() {
}

func (n *nodeExpandApplyableResource) References() []*addrs.Reference {
	refs := n.NodeAbstractResource.References()

	// The expand node needs to connect to the individual resource instances it
	// references, but cannot refer to it's own instances without causing
	// cycles. It would be preferable to entirely disallow self references
	// without the `self` identifier, but those were allowed in provisioners
	// for compatibility with legacy configuration. We also can't always just
	// filter them out for all resource node types, because the only method we
	// have for catching certain invalid configurations are the cycles that
	// result from these inter-instance references.
	return filterSelfRefs(n.Addr.Resource, refs)
}

func (n *nodeExpandApplyableResource) Name() string {
	return n.NodeAbstractResource.Name() + " (expand)"
}

func (n *nodeExpandApplyableResource) Execute(ctx context.Context, evalCtx EvalContext, op walkOperation) tfdiags.Diagnostics {
	// Because ephemeral resources are not stored in plan or state, the apply expansion needs
	// to produce a new graph too.
	// Therefore, for ephemeral resources, we want to skip the logic from this method since
	// the same thing will be executed during nodeExpandApplyableResource.DynamicExpand.
	if n.Addr.Resource.Mode == addrs.EphemeralResourceMode {
		return nil
	}
	var diags tfdiags.Diagnostics
	expander := evalCtx.InstanceExpander()
	moduleInstances := expander.ExpandModule(n.Addr.Module)
	for _, module := range moduleInstances {
		evalCtx = evalCtx.WithPath(module)
		diags = diags.Append(n.writeResourceState(ctx, evalCtx, n.Addr.Resource.Absolute(module)))
	}

	return diags
}

func (n *nodeExpandApplyableResource) DynamicExpand(evalCtx EvalContext) (*Graph, error) {
	// All of the other resource types have their information stored in the plan, but not ephemeral.
	// Resource types with information in the plan have the associated instance nodes created during a
	// separate transformer (ie: DiffTransfomer).
	// For ephemeral resources, we need to create the instance nodes again during the apply graph
	// building.
	if n.Addr.Resource.Mode != addrs.EphemeralResourceMode {
		return nil, nil
	}
	g, diags := n.expandEphemeralResource(context.TODO(), evalCtx)
	return g, diags.ErrWithWarnings()
}

// expandEphemeralResource logic is mostly got from nodeExpandPlannableResource.DynamicExpand.
func (n *nodeExpandApplyableResource) expandEphemeralResource(ctx context.Context, evalCtx EvalContext) (*Graph, tfdiags.Diagnostics) {
	var (
		diags tfdiags.Diagnostics
		g     Graph
	)
	expander := evalCtx.InstanceExpander()
	moduleInstances := expander.ExpandModule(n.Addr.Module)

	// The above dealt with the expansion of the containing module, so now
	// we need to deal with the expansion of the resource itself across all
	// instances of the module.
	//
	// We'll gather up all of the leaf instances we learn about along the way
	// so that we can inform the checks subsystem of which instances it should
	// be expecting check results for, below.
	instAddrs := addrs.MakeSet[addrs.Checkable]()
	for _, module := range moduleInstances {
		resAddr := n.Addr.Resource.Absolute(module)
		expDiags := n.expandEphemeralInstances(ctx, evalCtx, resAddr, &g, instAddrs)
		diags = diags.Append(expDiags)
	}
	if diags.HasErrors() {
		return nil, diags
	}

	// If this is a resource that participates in custom condition checks
	// (i.e. it has preconditions or postconditions) then the check state
	// wants to know the addresses of the checkable objects so that it can
	// treat them as unknown status if we encounter an error before actually
	// visiting the checks.
	if checkState := evalCtx.Checks(); checkState.ConfigHasChecks(n.NodeAbstractResource.Addr) {
		checkState.ReportCheckableObjects(n.NodeAbstractResource.Addr, instAddrs)
	}

	addRootNodeToGraph(&g)
	return &g, diags
}

// expandEphemeralInstances calculates the dynamic expansion for the ephemeral resource
// itself in the context of a particular module instance.
//
// It has several side-effects:
//   - Adds a node to Graph g for each leaf resource instance it discovers.
//   - Registers the expansion of the resource in the "expander" object embedded inside EvalContext ctx.
//   - Adds each present (non-orphaned) resource instance address to instAddrs (guaranteed to always be addrs.AbsResourceInstance, despite being declared as addrs.Checkable).
//
// After calling this for each of the module instances the resource appears
// within, the caller must register the final superset instAddrs with the
// checks subsystem so that it knows the fully expanded set of checkable
// object instances for this resource instance.
func (n *nodeExpandApplyableResource) expandEphemeralInstances(ctx context.Context, globalCtx EvalContext, resAddr addrs.AbsResource, g *Graph, instAddrs addrs.Set[addrs.Checkable]) (diags tfdiags.Diagnostics) {
	// The rest of our work here needs to know which module instance it's
	// working in, so that it can evaluate expressions in the appropriate scope.
	moduleCtx := globalCtx.WithPath(resAddr.Module)

	// writeResourceState is responsible for informing the expander of what
	// repetition mode this resource has, which allows expander.ExpandResource
	// to work below.
	moreDiags := n.writeResourceState(ctx, moduleCtx, resAddr)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return diags
	}

	expander := moduleCtx.InstanceExpander()
	instanceAddrs := expander.ExpandResource(resAddr)

	for _, addr := range instanceAddrs {
		// If this resource is participating in the "checks" mechanism then our
		// caller will need to know all of our expanded instance addresses as
		// checkable object instances.
		// (NOTE: instAddrs probably already has other instance addresses in it
		// from earlier calls to this function with different resource addresses,
		// because its purpose is to aggregate them all together into a single set.)
		instAddrs.Add(addr)
	}

	// Our graph builder mechanism expects to always be constructing new
	// graphs rather than adding to existing ones, so we'll first
	// construct a subgraph just for this individual modules's instances and
	// then we'll steal all of its nodes and edges to incorporate into our
	// main graph which contains all of the resource instances together.
	instG, err := n.ephemeralInstanceSubgraph(ctx, resAddr, instanceAddrs)
	if err != nil {
		diags = diags.Append(err)
		return diags
	}
	g.Subsume(&instG.AcyclicGraph.Graph)

	return diags
}

// ephemeralInstanceSubgraph creates the subgraph for the given ephemeral resource in the context of
// a module.
// This also records the closableResource.Close method into nodeExpandApplyableResource.closers to be later used
// to close the ephemeral resources.
func (n *nodeExpandApplyableResource) ephemeralInstanceSubgraph(ctx context.Context, addr addrs.AbsResource, instanceAddrs []addrs.AbsResourceInstance) (*Graph, error) {
	var diags tfdiags.Diagnostics

	// The concrete resource factory we'll use
	concreteResource := func(a *NodeAbstractResourceInstance) dag.Vertex {
		var m *NodeApplyableResourceInstance

		// Add the config and state since we don't do that via transforms
		a.Config = n.Config
		a.ResolvedProvider = n.ResolvedProvider
		a.Schema = n.Schema
		a.ProvisionerSchemas = n.ProvisionerSchemas
		a.ProviderMetas = n.ProviderMetas
		a.dependsOn = n.dependsOn
		a.generateConfigPath = n.generateConfigPath

		m = &NodeApplyableResourceInstance{
			NodeAbstractResourceInstance: a,
		}
		// When creating concrete instance nodes for the ephemeral resources we want to collect all the
		// resourceCloser callbacks from the nodes to be able to close the resources at the end of the graph walk.
		n.closers = append(n.closers, m.Close)

		return m
	}

	// Start creating the steps
	steps := []GraphTransformer{
		// Expand the count or for_each (if present)
		&ResourceCountTransformer{
			Concrete:      concreteResource,
			Schema:        n.Schema,
			Addr:          n.ResourceAddr(),
			InstanceAddrs: instanceAddrs,
		},

		// Targeting
		&TargetingTransformer{Targets: n.Targets, Excludes: n.Excludes},

		// Connect references so ordering is correct
		&ReferenceTransformer{},

		// Make sure there is a single root
		&RootTransformer{},
	}

	// Build the graph
	b := &BasicGraphBuilder{
		Steps: steps,
		Name:  "nodeExpandEphemeralApplyableResource",
	}
	graph, graphDiags := b.Build(ctx, addr.Module)
	return graph, diags.Append(graphDiags).ErrWithWarnings()
}

// Close implements closableResource
func (n *nodeExpandApplyableResource) Close() (diags tfdiags.Diagnostics) {
	if n.Addr.Resource.Mode != addrs.EphemeralResourceMode {
		return diags
	}

	var wg sync.WaitGroup
	diagsCh := make(chan tfdiags.Diagnostics, len(n.closers))
	log.Printf("[TRACE] nodeExpandApplyableResource - scheduling %d closing operations for of ephemeral resource %s", len(n.closers), n.Addr.String())
	for _, cb := range n.closers {
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
