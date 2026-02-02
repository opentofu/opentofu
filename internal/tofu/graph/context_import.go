package graph

import (
	"context"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu/contract"
)

func (c *Context) Import(ctx context.Context, config *configs.Config, prevRunState *states.State, opts *contract.ImportOpts) (*states.State, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Hold a lock since we can modify our own state here
	defer c.acquireRun("import")()

	// Don't modify our caller's state
	state := prevRunState.DeepCopy()

	log.Printf("[DEBUG] Building and walking import graph")

	variables := opts.SetVariables

	providerFunctionTracker := make(ProviderFunctionMapping)

	// Initialize our graph builder
	builder := &PlanGraphBuilder{
		ImportTargets:           opts.Targets,
		Config:                  config,
		State:                   state,
		RootVariableValues:      variables,
		Plugins:                 c.plugins,
		Operation:               walkImport,
		ProviderFunctionTracker: providerFunctionTracker,
	}

	// Build the graph
	graph, graphDiags := builder.Build(ctx, addrs.RootModuleInstance)
	diags = diags.Append(graphDiags)
	if graphDiags.HasErrors() {
		return state, diags
	}

	// Walk it
	walker, walkDiags := c.walk(ctx, graph, walkImport, &graphWalkOpts{
		Config:                  config,
		InputState:              state,
		ProviderFunctionTracker: providerFunctionTracker,
	})
	diags = diags.Append(walkDiags)
	if walkDiags.HasErrors() {
		return state, diags
	}

	// Once we have all instances expanded, we are able to do a complete validation for import targets
	// This part validates imports of both types (import blocks and CLI imports)
	allInstances := walker.InstanceExpander.AllInstances()
	importValidateDiags := c.postExpansionImportValidation(walker.ImportResolver, allInstances)
	if importValidateDiags.HasErrors() {
		return nil, importValidateDiags
	}

	// Data sources which could not be read during the import plan will be
	// unknown. We need to strip those objects out so that the state can be
	// serialized.
	walker.State.RemovePlannedResourceInstanceObjects()

	newState := walker.State.Close()
	return newState, diags
}
