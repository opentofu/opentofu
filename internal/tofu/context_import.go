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
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu/importing"
	"github.com/opentofu/opentofu/internal/tofu/variables"
)

// ImportOpts are used as the configuration for Import.
type ImportOpts struct {
	// Targets are the targets to import
	Targets []*importing.ImportTarget

	// SetVariables are the variables set outside of the configuration,
	// such as on the command line, in variables files, etc.
	SetVariables variables.InputValues
}

// Import takes already-created external resources and brings them
// under OpenTofu management. Import requires the exact type, name, and ID
// of the resources to import.
//
// This operation is idempotent. If the requested resource is already
// imported, no changes are made to the state.
//
// Further, this operation also gracefully handles partial state. If during
// an import there is a failure, all previously imported resources remain
// imported.
func (c *Context) Import(ctx context.Context, config *configs.Config, prevRunState *states.State, opts *ImportOpts) (*states.State, tfdiags.Diagnostics) {
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
