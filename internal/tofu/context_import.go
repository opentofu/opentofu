// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/instances"
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

// IsFromImportBlock checks whether the import target originates from an `import` block
// Currently, it should yield the opposite result of IsFromImportCommandLine, as those two are mutually-exclusive
func (i *ImportTarget) IsFromImportBlock() bool {
	return i.Config != nil
}

// IsFromImportCommandLine checks whether the import target originates from a `tofu import` command
// Currently, it should yield the opposite result of IsFromImportBlock, as those two are mutually-exclusive
func (i *ImportTarget) IsFromImportCommandLine() bool {
	return i.CommandLineImportTarget != nil
}

// StaticAddr returns the static address part of an import target
// For an ImportTarget originating from the command line, the address is already known
// However for an ImportTarget originating from an import block, the full address might not be known initially,
// and could only be evaluated down the line. Here, we create a static representation for the address.
// This is useful so that we could have information on the ImportTarget early on, such as the Module and Resource of it
func (i *ImportTarget) StaticAddr() addrs.ConfigResource {
	if i.IsFromImportCommandLine() {
		return i.CommandLineImportTarget.Addr.ConfigResource()
	}

	return i.Config.StaticTo
}

// ResolvedAddr returns a reference to the resolved address of an import target, if possible. If not possible, it
// returns nil.
// For an ImportTarget originating from the command line, the address is already known
// However for an ImportTarget originating from an import block, the full address might not be known initially,
// and could only be evaluated down the line.
func (i *ImportTarget) ResolvedAddr() *addrs.AbsResourceInstance {
	if i.IsFromImportCommandLine() {
		return &i.CommandLineImportTarget.Addr
	} else {
		return i.Config.ResolvedTo
	}
}

// ImportResolver is a struct that maintains a map of all imports as they are being resolved.
// This is specifically for imports originating from configuration.
// Import targets' addresses are not fully known from the get-go, and could only be resolved later when walking
// the graph. This struct helps keep track of the resolved imports, mostly for validation that all imports
// have been addressed and point to an actual configuration.
// The key of the map is a string representation of the address, and the value is an EvaluatedConfigImportTarget.
type ImportResolver struct {
	mu      sync.RWMutex
	imports map[string]EvaluatedConfigImportTarget
}

func NewImportResolver() *ImportResolver {
	return &ImportResolver{imports: make(map[string]EvaluatedConfigImportTarget)}
}

// ExpandAndResolveImport is responsible for two operations:
// 1. Expands the ImportTarget (originating from an import block) if it contains a 'for_each' attribute.
// 2. Goes over the expanded imports and resolves the ID and address, when we have the context necessary to resolve
// them. The resolved import target would be an EvaluatedConfigImportTarget.
// This function mutates the EvalContext's ImportResolver, adding the resolved import target.
// The function errors if we failed to evaluate the ID or the address.
func (ri *ImportResolver) ExpandAndResolveImport(importTarget *ImportTarget, ctx EvalContext) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	// The import block expressions are declared within the root module.
	// We need to explicitly use the context with the path of the root module, so that all references will be
	// relative to the root module
	rootCtx := ctx.WithPath(addrs.RootModuleInstance)

	if importTarget.Config.ForEach != nil {
		const unknownsNotAllowed = false
		const tupleAllowed = true

		// The import target has a for_each attribute, so we need to expand it
		forEachVal, evalDiags := evaluateForEachExpressionValue(context.TODO(), importTarget.Config.ForEach, rootCtx, unknownsNotAllowed, tupleAllowed, nil)
		diags = diags.Append(evalDiags)
		if diags.HasErrors() {
			return diags
		}

		// We are building an instances.RepetitionData based on each for_each key and val combination
		var repetitions []instances.RepetitionData

		it := forEachVal.ElementIterator()
		for it.Next() {
			k, v := it.Element()
			repetitions = append(repetitions, instances.RepetitionData{
				EachKey:   k,
				EachValue: v,
			})
		}

		for _, keyData := range repetitions {
			diags = diags.Append(ri.resolveImport(importTarget, rootCtx, keyData))
		}
	} else {
		// The import target is singular, no need to expand
		diags = diags.Append(ri.resolveImport(importTarget, rootCtx, EvalDataForNoInstanceKey))
	}

	return diags
}

// resolveImport resolves the ID and address of an ImportTarget originating from an import block,
// when we have the context necessary to resolve them. The resolved import target would be an
// EvaluatedConfigImportTarget.
// This function mutates the EvalContext's ImportResolver, adding the resolved import target.
// The function errors if we failed to evaluate the ID or the address.
func (ri *ImportResolver) resolveImport(importTarget *ImportTarget, ctx EvalContext, keyData instances.RepetitionData) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	importId, evalDiags := evaluateImportIdExpression(importTarget.Config.ID, ctx, keyData)
	diags = diags.Append(evalDiags)
	if diags.HasErrors() {
		return diags
	}

	importAddress, addressDiags := evaluateImportAddress(ctx, importTarget.Config.To, keyData)
	diags = diags.Append(addressDiags)
	if diags.HasErrors() {
		return diags
	}

	ri.mu.Lock()
	defer ri.mu.Unlock()

	resolvedImportKey := importAddress.String()

	if importTarget, exists := ri.imports[resolvedImportKey]; exists {
		return diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Duplicate import configuration for %q", importAddress),
			Detail:   fmt.Sprintf("An import block for the resource %q was already declared at %s. A resource can have only one import block.", importAddress, importTarget.Config.DeclRange),
			Subject:  importTarget.Config.DeclRange.Ptr(),
		})
	}

	ri.imports[resolvedImportKey] = EvaluatedConfigImportTarget{
		Config: importTarget.Config,
		Addr:   importAddress,
		ID:     importId,
	}

	if keyData == EvalDataForNoInstanceKey {
		log.Printf("[TRACE] importResolver: resolved a singular import target %s", importAddress)
	} else {
		log.Printf("[TRACE] importResolver: resolved an expanded import target %s", importAddress)
	}

	return diags
}

// GetAllImports returns all resolved imports
func (ri *ImportResolver) GetAllImports() []EvaluatedConfigImportTarget {
	ri.mu.RLock()
	defer ri.mu.RUnlock()

	var allImports []EvaluatedConfigImportTarget
	for _, importTarget := range ri.imports {
		allImports = append(allImports, importTarget)
	}
	return allImports
}

func (ri *ImportResolver) GetImport(address addrs.AbsResourceInstance) *EvaluatedConfigImportTarget {
	ri.mu.RLock()
	defer ri.mu.RUnlock()

	for _, importTarget := range ri.imports {
		if importTarget.Addr.Equal(address) {
			return &importTarget
		}
	}
	return nil
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

	// Data sources which could not be read during the import plan will be
	// unknown. We need to strip those objects out so that the state can be
	// serialized.
	walker.State.RemovePlannedResourceInstanceObjects()

	newState := walker.State.Close()
	return newState, diags
}
