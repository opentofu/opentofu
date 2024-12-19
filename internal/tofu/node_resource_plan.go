// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"iter"
	"log"
	"strings"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/dag"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// nodeExpandPlannableResource represents a static resource from the configuration,
// before it has been expanded into multiple instances.
//
// It's responsible for determining the final set of instances for the resource
// and then performing the planning logic for each one.
type nodeExpandPlannableResource struct {
	*NodeAbstractResource

	// ForceCreateBeforeDestroy might be set via our GraphNodeDestroyerCBD
	// during graph construction, if dependencies require us to force this
	// on regardless of what the configuration says.
	ForceCreateBeforeDestroy *bool

	// skipRefresh indicates that we should skip refreshing individual instances
	skipRefresh bool

	preDestroyRefresh bool

	// skipPlanChanges indicates we should skip trying to plan change actions
	// for any instances.
	skipPlanChanges bool

	// forceReplace are resource instance addresses where the user wants to
	// force generating a replace action. This set isn't pre-filtered, so
	// it might contain addresses that have nothing to do with the resource
	// that this node represents, which the node itself must therefore ignore.
	forceReplace []addrs.AbsResourceInstance

	// We attach dependencies to the Resource during refresh, since the
	// instances are instantiated during DynamicExpand.
	// FIXME: These would be better off converted to a generic Set data
	// structure in the future, as we need to compare for equality and take the
	// union of multiple groups of dependencies.
	dependencies []addrs.ConfigResource
}

var (
	_ GraphNodeDestroyerCBD         = (*nodeExpandPlannableResource)(nil)
	_ GraphNodeExecutable           = (*nodeExpandPlannableResource)(nil)
	_ GraphNodeReferenceable        = (*nodeExpandPlannableResource)(nil)
	_ GraphNodeReferencer           = (*nodeExpandPlannableResource)(nil)
	_ GraphNodeConfigResource       = (*nodeExpandPlannableResource)(nil)
	_ GraphNodeAttachResourceConfig = (*nodeExpandPlannableResource)(nil)
	_ GraphNodeAttachDependencies   = (*nodeExpandPlannableResource)(nil)
	_ GraphNodeTargetable           = (*nodeExpandPlannableResource)(nil)
	_ graphNodeExpandsInstances     = (*nodeExpandPlannableResource)(nil)
)

func (n *nodeExpandPlannableResource) Name() string {
	return n.NodeAbstractResource.Name() + " (expand)"
}

func (n *nodeExpandPlannableResource) expandsInstances() {
}

// GraphNodeAttachDependencies
func (n *nodeExpandPlannableResource) AttachDependencies(deps []addrs.ConfigResource) {
	n.dependencies = deps
}

// GraphNodeDestroyerCBD
func (n *nodeExpandPlannableResource) CreateBeforeDestroy() bool {
	if n.ForceCreateBeforeDestroy != nil {
		return *n.ForceCreateBeforeDestroy
	}

	// If we have no config, we just assume no
	if n.Config == nil || n.Config.Managed == nil {
		return false
	}

	return n.Config.Managed.CreateBeforeDestroy
}

// GraphNodeDestroyerCBD
func (n *nodeExpandPlannableResource) ModifyCreateBeforeDestroy(v bool) error {
	n.ForceCreateBeforeDestroy = &v
	return nil
}

func (n *nodeExpandPlannableResource) ExecuteOld(ctx EvalContext, op walkOperation) tfdiags.Diagnostics {
	// This was originally an implementation of GraphNodeDynamicExpandable, which we're
	// moving away from in favor of more normal-looking code that just deals with the
	// nested resource instances inline.
	// However, this particular one was building its "subgraph" across multiple different
	// functions and so is too risky to migrate all in one go, and so as an interim step
	// we're still constructing a "Graph" but then just pulling the vertices out of it
	// to execute directly. This lets us adopt the new style without significantly rewriting
	// this code at first, so we can clean this up gradually over several smaller changes.
	// FIXME: Remove all of the Graph machinery from this and just directly iterate over
	// the instances and take the necessary actions directly, instead of this confusing
	// inversion-of-control style.
	var g Graph

	expander := ctx.InstanceExpander()
	moduleInstances := expander.ExpandModule(n.Addr.Module)

	// Lock the state while we inspect it
	state := ctx.State().Lock()

	var orphans []*states.Resource
	for _, res := range state.Resources(n.Addr) {
		found := false
		for _, m := range moduleInstances {
			if m.Equal(res.Addr.Module) {
				found = true
				break
			}
		}
		// The module instance of the resource in the state doesn't exist
		// in the current config, so this whole resource is orphaned.
		if !found {
			orphans = append(orphans, res)
		}
	}

	// We'll no longer use the state directly here, and the other functions
	// we'll call below may use it so we'll release the lock.
	state = nil
	ctx.State().Unlock()

	// The concrete resource factory we'll use for orphans
	concreteResourceOrphan := func(a *NodeAbstractResourceInstance) *NodePlannableResourceInstanceOrphan {
		// Add the config and state since we don't do that via transforms
		a.Config = n.Config
		a.ResolvedProvider = n.ResolvedProvider
		// ResolvedProviderKey set in AttachResourceState
		a.Schema = n.Schema
		a.ProvisionerSchemas = n.ProvisionerSchemas
		a.ProviderMetas = n.ProviderMetas
		a.Dependencies = n.dependencies

		return &NodePlannableResourceInstanceOrphan{
			NodeAbstractResourceInstance: a,
			skipRefresh:                  n.skipRefresh,
			skipPlanChanges:              n.skipPlanChanges,
		}
	}

	for _, res := range orphans {
		for key := range res.Instances {
			addr := res.Addr.Instance(key)
			abs := NewNodeAbstractResourceInstance(addr)
			abs.AttachResourceState(res)
			n := concreteResourceOrphan(abs)
			g.Add(n)
		}
	}

	// Resolve addresses and IDs of all import targets that originate from import blocks
	// We do it here before expanding the resources in the modules, to avoid running this resolution multiple times
	importResolver := ctx.ImportResolver()
	var diags tfdiags.Diagnostics
	for _, importTarget := range n.importTargets {
		// If the import target originates from the import command (instead of the import block), we don't need to
		// resolve the import as it's already in the resolved form
		// In addition, if PreDestroyRefresh is true, we know we are running as part of a refresh plan, immediately before a destroy
		// plan. In the destroy plan mode, import blocks are not relevant, that's why we skip resolving imports
		skipImports := importTarget.IsFromImportBlock() && !n.preDestroyRefresh
		if skipImports {
			err := importResolver.ExpandAndResolveImport(importTarget, ctx)
			diags = diags.Append(err)
		}
	}

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
		err := n.expandResourceInstances(ctx, resAddr, &g, instAddrs)
		diags = diags.Append(err)
	}
	if diags.HasErrors() {
		return diags
	}

	// If this is a resource that participates in custom condition checks
	// (i.e. it has preconditions or postconditions) then the check state
	// wants to know the addresses of the checkable objects so that it can
	// treat them as unknown status if we encounter an error before actually
	// visiting the checks.
	if checkState := ctx.Checks(); checkState.ConfigHasChecks(n.NodeAbstractResource.Addr) {
		checkState.ReportCheckableObjects(n.NodeAbstractResource.Addr, instAddrs)
	}

	// By the time we get here we should have a graph that has a node for
	// each resource instance, including both some added directly above
	// for the whole-module "orphans" and some added by the
	// n.expandResourceInstances method that still thinks it's building
	// real graphs.
	//
	// Because nodeExpandPlannableResource.expandResourceInstances is still
	// using the "graph builder" infrastructure to do its work, there will
	// unfortunately be some needless dependency edges in this graph between
	// each real node and the inert root node that is required for a graph
	// to be considered valid, and so we'll just ignore all the edges and
	// ignore any non-executable nodes so that we will effectively skip the
	// "root" node.
	//
	// FIXME: Tear all of the graph-building bureaucracy out of this and
	// iterate over the instances using a normal loop, without
	// inversion-of-control techiques.
	seq := iter.Seq[GraphNodeExecutable](func(yield func(GraphNodeExecutable) bool) {
		for _, v := range g.Vertices() {
			node, ok := v.(GraphNodeExecutable)
			if !ok {
				// We'll get in here for the unnecessary extra "root" node that the
				// graph builder inserts to make the graph valid. It has no actual
				// behavior, so we can safely skip it.
				continue
			}
			if !yield(node) {
				break
			}
		}
	})
	moreDiags := executeGraphNodes(seq, ctx, op)
	diags = diags.Append(moreDiags)
	return diags
}

func (n *nodeExpandPlannableResource) Execute(evalCtx EvalContext, op walkOperation) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	// Resolve addresses and IDs of all import targets that originate from import blocks
	// We do it here before expanding the resources in the modules, to avoid running this resolution multiple times
	importResolver := evalCtx.ImportResolver()
	for _, importTarget := range n.importTargets {
		// If the import target originates from the import command (instead of the import block), we don't need to
		// resolve the import as it's already in the resolved form
		// In addition, if PreDestroyRefresh is true, we know we are running as part of a refresh plan, immediately before a destroy
		// plan. In the destroy plan mode, import blocks are not relevant, that's why we skip resolving imports
		skipImports := importTarget.IsFromImportBlock() && !n.preDestroyRefresh
		if skipImports {
			resolveDiags := importResolver.ExpandAndResolveImport(importTarget, evalCtx)
			diags = diags.Append(resolveDiags)
		}
	}

	// We have two levels of "expansion" to deal with here: the module(s) we are
	// contained within might have expansions themselves and the resource can
	// also declare its own expansion arguments.
	// Module expansion should already have been resolved by an upstream
	// graph node, but it's this node's responsibility to finalize the
	// expansion of the resource itself across all module instances it
	// is defined in.
	expander := evalCtx.InstanceExpander()
	moduleInstances := expander.ExpandModule(n.Addr.Module)
	for _, moduleInst := range moduleInstances {
		moduleEvalCtx := evalCtx.WithPath(moduleInst)
		resourceAddr := n.Addr.Absolute(moduleInst)

		// This method calculates and registers the expansion for this
		// resource inside moduleInst as a side-effect, along with
		// also ensuring that other resource-related metadata is
		// recorded in the working state.
		stateDiags := n.writeResourceState(moduleEvalCtx, resourceAddr)
		diags = diags.Append(stateDiags)
	}

	// If any of the expansions failed then we can't proceed because
	// the expander would panic if asked to expand a resource that
	// hasn't had its instances registered by n.writeResourceState above.
	// This also returns early if any of the import resolves failed,
	// since otherwise we might make an unsuitable plan for a resource
	// instance that was intended to be an import target.
	if diags.HasErrors() {
		return diags
	}

	// We should now have enough information to decide all of the resource
	// instance objects we need to generate plans for.
	toPlan, toPlanDiags := n.objectsToPlan(evalCtx.State(), expander, importResolver)
	diags = diags.Append(toPlanDiags)
	if toPlanDiags.HasErrors() {
		return diags
	}

	// If this resource has any checks associated with it then we need to now
	// report all of the instances that are in the desired state to the
	// checks system so that it can know they are expected and can report
	// their results as unknown if we fail partway through evaluation.
	if checkState := evalCtx.Checks(); checkState.ConfigHasChecks(n.NodeAbstractResource.Addr) {
		checkableInstAddrs := addrs.MakeSet[addrs.Checkable]()
		for _, elem := range toPlan.Elems {
			instAddr := elem.Key
			obj := elem.Value[states.NotDeposed]
			if obj != nil && obj.Config != nil {
				// Non-nil config means that this object is in the desired state.
				checkableInstAddrs.Add(instAddr)
			}
		}
		checkState.ReportCheckableObjects(n.NodeAbstractResource.Addr, checkableInstAddrs)
	}

	// With all of that book-keeping done, we are now ready to actually
	// create plans for all of the objects we've identified. This directly
	// modifies the "changes" object to include the new planned changes.
	planDiags := n.planAllObjects(toPlan, evalCtx, op)
	diags = diags.Append(planDiags)

	return diags
}

func (n *nodeExpandPlannableResource) planAllObjects(toPlan resourceInstanceObjectsToPlan, evalCtx EvalContext, op walkOperation) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	// For now the actual planning functionality still lives in separate "graph node" types,
	// and so our work here is to translate each element of toPlan into one of those
	// so that our executeGraphNodes shim can consume it.
	// TODO: Replace each of these "graph node" types with just a normal function we can
	// call directly here.
	graphNodes := func(yield func(GraphNodeExecutable) bool) {
		for _, elem := range toPlan.Elems {
			instAddr := elem.Key

			// TEMP: The resourceInstanceObjectToPlan structure is designed for an end-state
			// where each object is handled separately and so it includes only a
			// ResourceInstanceObject state, but these legacy graph nodes are all designed
			// as if they are working at whole-resource-instance granularity and so
			// expect to be given the whole resource instance state and then pick out
			// the object they are working with internally.
			//
			// To compensate for that while we're in this intermediate phase of still using
			// the "graph node" types despite not actually making a graph, we'll pluck
			// the whole instance state directly out of the overall working state object
			// here for now.
			instanceState := evalCtx.State().ResourceInstance(instAddr)

			for deposedKey, obj := range elem.Value {
				if deposedKey == states.NotDeposed {
					log.Printf("[TRACE] nodePlannableResource: need to plan %s", instAddr)
				} else {
					log.Printf("[TRACE] nodePlannableResource: need to plan %s deposed object %s", instAddr, deposedKey)
				}
				abstract := &NodeAbstractResourceInstance{
					NodeAbstractResource: *n.NodeAbstractResource,
					Addr:                 instAddr,
					Dependencies:         n.dependencies,
					ResolvedProviderKey:  obj.Provider.KeyExact, // FIXME: This is not the correct way to populate this field
					instanceState:        instanceState,
					preDestroyRefresh:    n.preDestroyRefresh,
				}

				var node GraphNodeExecutable
				switch {
				case obj.CommandLineImportTarget != nil:
					// For command-line import targets (i.e. from the "tofu import" command)
					// we use a separate node type entirely, rather than handling it just
					// as part of the normal instance planning.
					node = &graphNodeImportState{
						Addr:             abstract.Addr,
						ID:               obj.CommandLineImportTarget.ID,
						ResolvedProvider: n.ResolvedProvider,
						Schema:           n.Schema,
						SchemaVersion:    n.SchemaVersion,
						Config:           n.Config,
					}

				case obj.Config != nil:

					// This object is part of the desired state, so we'll plan it
					// as a NodePlannableResourceInstance.
					node = &NodePlannableResourceInstance{
						NodeAbstractResourceInstance: abstract,
						ForceCreateBeforeDestroy:     n.CreateBeforeDestroy(),
						skipRefresh:                  n.skipRefresh,
						skipPlanChanges:              n.skipPlanChanges,
						forceReplace:                 n.forceReplace, // TODO: Use obj.ForceReplace directly instead in future
					}
					if obj.ConfigImportTarget != nil {
						// TODO: Why isn't this importTarget field also a pointer, so we could
						// just copy it over without deref? That answer might be relevant when we
						// rework this to be just a normal function instead of a graph node
						// in future.
						node.(*NodePlannableResourceInstance).importTarget = *obj.ConfigImportTarget
					}

				case deposedKey == states.NotDeposed:
					// A non-deposed object without a configuration is not part of
					// the desired state and so we always want to plan to destroy it.
					// For that we use NodePlannableResourceInstanceOrphan.
					node = &NodePlannableResourceInstanceOrphan{
						NodeAbstractResourceInstance: abstract,
						skipRefresh:                  n.skipRefresh,
						skipPlanChanges:              n.skipPlanChanges,
					}

				default:
					// If we get here then we have a non-desired object that is deposed,
					// which needs special handling in NodePlanDeposedResourceInstanceObject.
					node = &NodePlanDeposedResourceInstanceObject{
						NodeAbstractResourceInstance: abstract,
						DeposedKey:                   deposedKey,
						skipRefresh:                  n.skipRefresh,
						skipPlanChanges:              n.skipPlanChanges,
						// TODO: EndpointsToRemove
					}

				}
				if !yield(node) {
					return
				}
			}
		}
	}

	execDiags := executeGraphNodes(graphNodes, evalCtx, op)
	diags = diags.Append(execDiags)

	return diags
}

// expandResourceInstances calculates the dynamic expansion for the resource
// itself in the context of a particular module instance.
//
// It has several side-effects:
//   - Adds a node to Graph g for each leaf resource instance it discovers, whether present or orphaned.
//   - Registers the expansion of the resource in the "expander" object embedded inside EvalContext ctx.
//   - Adds each present (non-orphaned) resource instance address to instAddrs (guaranteed to always be addrs.AbsResourceInstance, despite being declared as addrs.Checkable).
//
// After calling this for each of the module instances the resource appears
// within, the caller must register the final superset instAddrs with the
// checks subsystem so that it knows the fully expanded set of checkable
// object instances for this resource instance.
func (n *nodeExpandPlannableResource) expandResourceInstances(globalCtx EvalContext, resAddr addrs.AbsResource, g *Graph, instAddrs addrs.Set[addrs.Checkable]) error {
	var diags tfdiags.Diagnostics

	// The rest of our work here needs to know which module instance it's
	// working in, so that it can evaluate expressions in the appropriate scope.
	moduleCtx := globalCtx.WithPath(resAddr.Module)

	// writeResourceState is responsible for informing the expander of what
	// repetition mode this resource has, which allows expander.ExpandResource
	// to work below.
	moreDiags := n.writeResourceState(moduleCtx, resAddr)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return diags.ErrWithWarnings()
	}

	// Before we expand our resource into potentially many resource instances,
	// we'll verify that any mention of this resource in n.forceReplace is
	// consistent with the repetition mode of the resource. In other words,
	// we're aiming to catch a situation where naming a particular resource
	// instance would require an instance key but the given address has none.
	expander := moduleCtx.InstanceExpander()
	instanceAddrs := expander.ExpandResource(resAddr)

	// If there's a number of instances other than 1 then we definitely need
	// an index.
	mustHaveIndex := len(instanceAddrs) != 1
	// If there's only one instance then we might still need an index, if the
	// instance address has one.
	if len(instanceAddrs) == 1 && instanceAddrs[0].Resource.Key != addrs.NoKey {
		mustHaveIndex = true
	}
	if mustHaveIndex {
		for _, candidateAddr := range n.forceReplace {
			if candidateAddr.Resource.Key == addrs.NoKey {
				if n.Addr.Resource.Equal(candidateAddr.Resource.Resource) {
					switch {
					case len(instanceAddrs) == 0:
						// In this case there _are_ no instances to replace, so
						// there isn't any alternative address for us to suggest.
						diags = diags.Append(tfdiags.Sourceless(
							tfdiags.Warning,
							"Incompletely-matched force-replace resource instance",
							fmt.Sprintf(
								"Your force-replace request for %s doesn't match any resource instances because this resource doesn't have any instances.",
								candidateAddr,
							),
						))
					case len(instanceAddrs) == 1:
						diags = diags.Append(tfdiags.Sourceless(
							tfdiags.Warning,
							"Incompletely-matched force-replace resource instance",
							fmt.Sprintf(
								"Your force-replace request for %s doesn't match any resource instances because it lacks an instance key.\n\nTo force replacement of the single declared instance, use the following option instead:\n  -replace=%q",
								candidateAddr, instanceAddrs[0],
							),
						))
					default:
						var possibleValidOptions strings.Builder
						for _, addr := range instanceAddrs {
							fmt.Fprintf(&possibleValidOptions, "\n  -replace=%q", addr)
						}

						diags = diags.Append(tfdiags.Sourceless(
							tfdiags.Warning,
							"Incompletely-matched force-replace resource instance",
							fmt.Sprintf(
								"Your force-replace request for %s doesn't match any resource instances because it lacks an instance key.\n\nTo force replacement of particular instances, use one or more of the following options instead:%s",
								candidateAddr, possibleValidOptions.String(),
							),
						))
					}
				}
			}
		}
	}
	// NOTE: The actual interpretation of n.forceReplace to produce replace
	// actions is in the per-instance function we're about to call, because
	// we need to evaluate it on a per-instance basis.

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
	instG, err := n.resourceInstanceSubgraph(moduleCtx, resAddr, instanceAddrs)
	if err != nil {
		diags = diags.Append(err)
		return diags.ErrWithWarnings()
	}
	g.Subsume(&instG.AcyclicGraph.Graph)

	return diags.ErrWithWarnings()
}

func (n *nodeExpandPlannableResource) resourceInstanceSubgraph(ctx EvalContext, addr addrs.AbsResource, instanceAddrs []addrs.AbsResourceInstance) (*Graph, error) {
	var diags tfdiags.Diagnostics

	var commandLineImportTargets []CommandLineImportTarget

	for _, importTarget := range n.importTargets {
		if importTarget.IsFromImportCommandLine() {
			commandLineImportTargets = append(commandLineImportTargets, *importTarget.CommandLineImportTarget)
		}
	}

	// Our graph transformers require access to the full state, so we'll
	// temporarily lock it while we work on this.
	state := ctx.State().Lock()
	defer ctx.State().Unlock()

	// The concrete resource factory we'll use
	concreteResource := func(a *NodeAbstractResourceInstance) dag.Vertex {
		var m *NodePlannableResourceInstance

		// If we're in the `tofu import` CLI command, we only need
		// to return the import node, not a plannable resource node.
		for _, c := range commandLineImportTargets {
			if c.Addr.Equal(a.Addr) {
				return &graphNodeImportState{
					Addr:             c.Addr,
					ID:               c.ID,
					ResolvedProvider: n.ResolvedProvider,
					Schema:           n.Schema,
					SchemaVersion:    n.SchemaVersion,
					Config:           n.Config,
				}
			}
		}

		// Add the config and state since we don't do that via transforms
		a.Config = n.Config
		a.ResolvedProvider = n.ResolvedProvider
		a.Schema = n.Schema
		a.ProvisionerSchemas = n.ProvisionerSchemas
		a.ProviderMetas = n.ProviderMetas
		a.dependsOn = n.dependsOn
		a.Dependencies = n.dependencies
		a.preDestroyRefresh = n.preDestroyRefresh
		a.generateConfigPath = n.generateConfigPath

		m = &NodePlannableResourceInstance{
			NodeAbstractResourceInstance: a,

			// By the time we're walking, we've figured out whether we need
			// to force on CreateBeforeDestroy due to dependencies on other
			// nodes that have it.
			ForceCreateBeforeDestroy: n.CreateBeforeDestroy(),
			skipRefresh:              n.skipRefresh,
			skipPlanChanges:          n.skipPlanChanges,
			forceReplace:             n.forceReplace,
		}

		resolvedImportTarget := ctx.ImportResolver().GetImport(a.Addr)
		if resolvedImportTarget != nil {
			m.importTarget = *resolvedImportTarget
		}

		return m
	}

	// The concrete resource factory we'll use for orphans
	concreteResourceOrphan := func(a *NodeAbstractResourceInstance) dag.Vertex {
		// Add the config and state since we don't do that via transforms
		a.Config = n.Config
		a.ResolvedProvider = n.ResolvedProvider
		// ResolvedProviderKey will be set during AttachResourceState
		a.Schema = n.Schema
		a.ProvisionerSchemas = n.ProvisionerSchemas
		a.ProviderMetas = n.ProviderMetas

		return &NodePlannableResourceInstanceOrphan{
			NodeAbstractResourceInstance: a,
			skipRefresh:                  n.skipRefresh,
			skipPlanChanges:              n.skipPlanChanges,
		}
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

		// Add the count/for_each orphans
		&OrphanResourceInstanceCountTransformer{
			Concrete:      concreteResourceOrphan,
			Addr:          addr,
			InstanceAddrs: instanceAddrs,
			State:         state,
		},

		// Attach the state
		&AttachStateTransformer{State: state},

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
		Name:  "nodeExpandPlannableResource",
	}
	graph, graphDiags := b.Build(addr.Module)
	return graph, diags.Append(graphDiags).ErrWithWarnings()
}

// objectsToPlan produces a sort of "plan for what to plan", by analyzing the current
// situation for all of the resource instance objects associated with this node's resource,
// finding the subset that need to be given the opportunity to create a plan, and
// gathering together the object-specific information required to actually create that
// plan as a subsequent step.
//
// Before calling this function the given [instances.Expander] must already have the
// expansion values registered for all of the module calls leading to this resource
// and for the resource itself, and the given [ImportResolver] must already have
// all of the import requests related to this resource registered with it.
//
// This is intended as the first of two phases, working only with data that we already
// have in memory. A subsequent step can use the result of this one to decide how to
// perform all of the I/O required to plan all of the described objects as efficiently
// as possible while respecting the user's configured concurrency limit.
//
//nolint:unused // work in progress
func (n *nodeExpandPlannableResource) objectsToPlan(stateSync *states.SyncState, expander *instances.Expander, importResolver *ImportResolver) (resourceInstanceObjectsToPlan, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	ret := addrs.MakeMap[addrs.AbsResourceInstance, map[states.DeposedKey]*resourceInstanceObjectToPlan]()

	// This method is intentionally written as a bunch of calls to
	// free functions that operate only on their direct arguments so
	// that we can more easily write tests for those individual functions
	// to cover their individual behaviors, rather than having to test
	// everything at once in all cases.

	forceReplace := addrs.MakeSet(n.forceReplace...)

	// The module that we're declared in might have multiple instances itself,
	// so we'll start by resolving those.
	moduleInstances := expander.ExpandModule(n.Addr.Module)

	// Import targets from the "tofu import" command need some special treatment.
	var commandLineImportTargets []CommandLineImportTarget
	for _, importTarget := range n.importTargets {
		if importTarget.IsFromImportCommandLine() {
			commandLineImportTargets = append(commandLineImportTargets, *importTarget.CommandLineImportTarget)
		}
	}

	// We need to hold the state lock just long enough to inspect the state
	// objects related to instances of our resource, which therefore avoids
	// the need to deep-copy those entries. Nothing should be altering the
	// prior state while we're planning, so we can safely make decisions
	// based on this information without retaining the lock.
	{
		// This is a nested scope to prevent accidentally using state or
		// perModuleStates below after we've released the lock.
		state := stateSync.Lock()
		perModuleStates := state.Resources(n.Addr)
		// Any resource instances that belong to module instances that
		// are no longer declared in the configuration need all of
		// their
		moreDiags := addResourceInstanceObjectsFromRemovedModuleInstances(moduleInstances, perModuleStates, n.ResolvedProvider, ret)
		stateSync.Unlock()
		diags = diags.Append(moreDiags)
	}

	// Now we'll deal with the resource instances that are associated with
	// the module instances that _are_ in the desired state.
	for _, moduleInstance := range moduleInstances {
		absResourceAddr := n.Addr.Absolute(moduleInstance)
		resourceState := stateSync.Resource(absResourceAddr)

		instanceAddrs := addrs.MakeSet(expander.ExpandResource(absResourceAddr)...)
		for _, instanceAddr := range instanceAddrs {
			priorState := stateSync.ResourceInstance(instanceAddr)       // nil for instances that didn't previously exist
			configImportTarget := importResolver.GetImport(instanceAddr) // nil for instances that aren't being imported into
			var commandLineImportTarget *CommandLineImportTarget
			for _, it := range commandLineImportTargets {
				if it.Addr.Equal(instanceAddr) {
					commandLineImportTarget = &it
					break
				}
			}
			moreDiags := addResourceInstanceObjectsForResourceInstance(
				instanceAddr,
				n.Config,
				priorState,
				configImportTarget,
				commandLineImportTarget,
				forceReplace.Has(instanceAddr),
				n.ResolvedProvider,
				ret,
			)
			diags = diags.Append(moreDiags)
		}

		// We also need to deal with any instances that are in the prior
		// state but not in the desired state, which therefore need a
		// "delete" or "forget" action planned for them.
		for instanceKey, priorState := range resourceState.Instances {
			instanceAddr := absResourceAddr.Instance(instanceKey)
			if instanceAddrs.Has(instanceAddr) {
				continue // This one is in the desired state so we already dealt with it above
			}
			moreDiags := addResourceInstanceObjectsForResourceInstance(
				absResourceAddr.Instance(instanceKey),
				nil, // indicates that the instance is not in the desired state
				priorState,
				nil, // can't import into a resource instance that isn't in the desired state
				nil, // can't import into a resource instance that isn't in the desired state
				forceReplace.Has(instanceAddr),
				n.ResolvedProvider,
				ret,
			)
			diags = diags.Append(moreDiags)
		}
	}

	return ret, diags
}

//nolint:unparam,unused // intentionally returning nil diagnostics so caller is prepared for future work
func addResourceInstanceObjectsForResourceInstance(
	addr addrs.AbsResourceInstance,
	config *configs.Resource, // nil for instances that aren't in the desired state
	priorState *states.ResourceInstance, // nil for instances that didn't previously exist
	configImportTarget *EvaluatedConfigImportTarget, // nil for instances that aren't being imported into by config
	commandLineImportTarget *CommandLineImportTarget, // nil for instances that aren't being imported into by command line
	forceReplace bool,
	provider ResolvedProvider,
	into resourceInstanceObjectsToPlan,
) tfdiags.Diagnostics {
	objs := ensureResourceInstanceToPlan(into, addr)

	var currentObj *states.ResourceInstanceObjectSrc
	var deposedObjs map[states.DeposedKey]*states.ResourceInstanceObjectSrc
	if priorState != nil {
		currentObj = priorState.Current
		deposedObjs = priorState.Deposed
	}

	// If either the instance is in the desired state or it has a current
	// object in the prior state then we need to plan it.
	if config != nil || currentObj != nil {
		objs[states.NotDeposed] = &resourceInstanceObjectToPlan{
			Addr:                    addr,
			DeposedKey:              states.NotDeposed,
			Config:                  config,
			PriorState:              currentObj,
			ConfigImportTarget:      configImportTarget,
			CommandLineImportTarget: commandLineImportTarget,
			ForceReplace:            forceReplace,
			// TODO: Removed?
			Provider: provider,
		}
	}

	// Any deposed objects are always included with no config to represent
	// that they are not desired and need to be planned for destruction.
	for deposedKey, stateObj := range deposedObjs {
		objs[deposedKey] = &resourceInstanceObjectToPlan{
			Addr:       addr,
			DeposedKey: deposedKey,
			Config:     nil, // deposed objects are never in the desired state
			PriorState: stateObj,
			Provider:   provider,
		}
	}

	return nil // We currently never generate diagnostics
}

//nolint:unparam // intentionally returning nil diagnostics so caller is prepared for future work
func addResourceInstanceObjectsFromRemovedModuleInstances(
	currentModuleInstances []addrs.ModuleInstance,
	priorStates []*states.Resource,
	provider ResolvedProvider,
	into resourceInstanceObjectsToPlan,
) tfdiags.Diagnostics {
States:
	for _, rs := range priorStates {
		for _, m := range currentModuleInstances {
			if m.Equal(rs.Addr.Module) {
				continue States
			}
		}
		// If there is no corresponding element in currentModuleInstances
		// then all of the existing objects under this module instance address
		// need to be planned for destruction, which we'll signal by generating
		// results that have no Config.
		for instKey, is := range rs.Instances {
			instAddr := rs.Addr.Instance(instKey)
			objs := ensureResourceInstanceToPlan(into, instAddr)
			if is.Current != nil {
				objs[states.NotDeposed] = &resourceInstanceObjectToPlan{
					Addr:       instAddr,
					PriorState: is.Current,
					Provider:   provider,
					// TODO: do we need to populate the Removed field?
				}
			}
			for deposedKey, os := range is.Deposed {
				objs[deposedKey] = &resourceInstanceObjectToPlan{
					Addr:       instAddr,
					PriorState: os,
					Provider:   provider,
				}
			}
		}
	}

	return nil // We currently never generate diagnostics
}

// resourceInstanceObjectsToPlan is a type alias for a specific map signature we use
// to describe a set of resource instance objects that ought to have planning performed
// against them.
//
// A value of this type could be thought of as a "plan for what to plan", which
// we construct first and then act on separately afterwards just because that leads to two
// phases that can be understood separately, rather than a single swamp of code dealing with
// both identifying what needs to be planned and doing the planning at the same time.
//
// This is aliased here just because the overall type signature is long and distracting
// when written repeatedly inline in functions working with this type.
type resourceInstanceObjectsToPlan = addrs.Map[
	addrs.AbsResourceInstance,
	map[states.DeposedKey]*resourceInstanceObjectToPlan,
]

func ensureResourceInstanceToPlan(into resourceInstanceObjectsToPlan, instAddr addrs.AbsResourceInstance) map[states.DeposedKey]*resourceInstanceObjectToPlan {
	if !into.Has(instAddr) {
		into.Put(instAddr, map[states.DeposedKey]*resourceInstanceObjectToPlan{})
	}
	return into.Get(instAddr)
}

// resourceInstanceObjectToPlan represents a single object from an instance of a resource that
// we've determined needs to have the planning process run against it.
//
// Different combinations of populated fields in this type represent different situations that
// are likely to require different handling in the planning process:
//
//   - If Config is nil while PriorState is non-nil then this object is not part of
//     the current desired state but existed in the prior state, and so we ought to plan to destroy
//     or forget it, taking into account any information in Removed.
//
//   - If PriorState is non-nil while Config is nil then this object has been added to
//     the desired state since the last plan/apply round and so we're presumably going to
//     plan to create it or import into it.
//
//   - If ImportTarget is set and PriorState is nil then we ought to plan to bind an existing
//     remote object to the given resource instance address.
//
//   - If ForceReplace is set then in any situation where we'd normally produce a no-op or
//     update action for this object we should produce a "replace" action instead.
type resourceInstanceObjectToPlan struct {
	// Addr is the full resource instance address that this object belongs to.
	Addr addrs.AbsResourceInstance

	// DeposedKey, if set to a value other than [states.NotDeposed], represents the non-current
	// object that needs to be planned.
	//
	// If this is [states.NotDeposed] then the intent is to plan the "current" object associated
	// with this resource instance.
	//
	// When DeposedKey is set, Config is always nil and ForceReplace is always false, because the
	// only reasonable action to take against a deposed object is to destroy it.
	DeposedKey states.DeposedKey

	// Config describes the configuration block for the resource that this is an instance of.
	//
	// This is nil if the resource block is no longer present in the configuration, or if
	// this object is describing a dynamic instance of a resource that was in the prior
	// state but is not included in the current configuration's desired state.
	Config *configs.Resource

	// PriorState describes the state for this resource instance object created in the previous
	// plan/apply round.
	//
	// This is nil if the resource instance was not declared in the previous round, in which
	// case this instance (or its whole resource block) have presumably been added to the
	// configuration since the last round.
	PriorState *states.ResourceInstanceObjectSrc

	// ConfigImportTarget is set if the configuration includes a request to import an existing
	// remote object into this resource instance address.
	//
	// This is nil if there is no import request for this resource instance.
	ConfigImportTarget *EvaluatedConfigImportTarget

	// CommandLineImportTarget is set if we're running "tofu import" and this is the object
	// that was identified as the import target.
	//
	// This is nil if this is either not "tofu import" at all or if this is not the object
	// selected as the import target.
	//
	// TODO: Can we find some way to combine this with ConfigImportTarget so that we can
	// encapsulate the handling of the two different kinds of imports into the
	// nodeExpandPlannableResource.objectsToPlan logic, and have the actual plan executor
	// handle them both in the same way?
	CommandLineImportTarget *CommandLineImportTarget

	// Removed describes any "removed" blocks associated with this resource instance.
	//
	// This is zero-length if there are no "removed" block targeting this resource instance.
	Removed []*configs.Removed

	// ForceReplace is set if this resource instance is subject to a "-replace" planning
	// option, which requests that any no-op or update action for this resource instance
	// should be treated as a "replace" instead.
	ForceReplace bool

	// Provider identifies the provider instance that should be used to create the plan for
	// this resource instance.
	Provider ResolvedProvider
}
