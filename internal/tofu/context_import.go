// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ImportOpts are used as the configuration for Import.
type ImportOpts struct {
	// Targets are the targets to import
	Targets []*ImportTarget

	// SetVariables are the variables set outside of the configuration,
	// such as on the command line, in variables files, etc.
	SetVariables InputValues
}

// CommandLineImportTarget is a target that we need to import, that originated from the CLI command
// It represents a single resource that we need to import.
// The resource's ID and Address are fully known when executing the command (unlike when using the `import` block)
type CommandLineImportTarget struct {
	// Addr is the address for the resource instance that the new object should
	// be imported into.
	Addr addrs.AbsResourceInstance

	// ID is the string ID of the resource to import. This is resource-specific.
	ID string
}

// ImportTarget is a target that we need to import.
// It could either represent a single resource or multiple instances of the same resource, if for_each is used
// ImportTarget can be either a result of the import CLI command, or the import block
type ImportTarget struct {
	// Config is the original import block for this import. This might be null
	// if the import did not originate in config.
	// Config is mutually-exclusive with CommandLineImportTarget
	Config *configs.Import

	// CommandLineImportTarget is the ImportTarget information in the case of an import target origination for the
	// command line. CommandLineImportTarget is mutually-exclusive with Config
	*CommandLineImportTarget
}

// StaticAddr returns the static address of an import target
// For an ImportTarget originating from the command line, the address is already known
// However for an ImportTarget originating from an import block, the full address might not be known initially,
// and could only be evaluated down the line. Here, we create a static representation for the address.
// This is useful so that we could have information on the ImportTarget early on, such as the Module and Resource of it
func (i *ImportTarget) StaticAddr() addrs.ConfigResource {
	if i.CommandLineImportTarget != nil {
		return i.CommandLineImportTarget.Addr.ConfigResource()
	}

	// TODO change this later, once we change Config.To to not be a static address
	return i.Config.To.ConfigResource()
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
func (c *Context) Import(config *configs.Config, prevRunState *states.State, opts *ImportOpts) (*states.State, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Hold a lock since we can modify our own state here
	defer c.acquireRun("import")()

	// Don't modify our caller's state
	state := prevRunState.DeepCopy()

	log.Printf("[DEBUG] Building and walking import graph")

	variables := opts.SetVariables

	// Initialize our graph builder
	builder := &PlanGraphBuilder{
		ImportTargets:      opts.Targets,
		Config:             config,
		State:              state,
		RootVariableValues: variables,
		Plugins:            c.plugins,
		Operation:          walkImport,
	}

	// Build the graph
	graph, graphDiags := builder.Build(addrs.RootModuleInstance)
	diags = diags.Append(graphDiags)
	if graphDiags.HasErrors() {
		return state, diags
	}

	// Walk it
	walker, walkDiags := c.walk(graph, walkImport, &graphWalkOpts{
		Config:     config,
		InputState: state,
	})
	diags = diags.Append(walkDiags)
	if walkDiags.HasErrors() {
		return state, diags
	}

	// Data sources which could not be read during the import plan will be
	// unknown. We need to strip those objects out so that the state can be
	// serialized.
	walker.State.RemovePlannedResourceInstanceObjects()

	newState := walker.State.Close()
	return newState, diags
}
