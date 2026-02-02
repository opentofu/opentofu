// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package graph

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/refactoring"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu/importing"
)

// graphWalkOpts captures some transient values we use (and possibly mutate)
// during a graph walk.
//
// The way these options get used unfortunately varies between the different
// walkOperation types. This is a historical design wart that dates back to
// us using the same graph structure for all operations; hopefully we'll
// make the necessary differences between the walk types more explicit someday.
type graphWalkOpts struct {
	InputState *states.State
	Changes    *plans.Changes
	Config     *configs.Config

	// PlanTimeCheckResults should be populated during the apply phase with
	// the snapshot of check results that was generated during the plan step.
	//
	// This then propagates the decisions about which checkable objects exist
	// from the plan phase into the apply phase without having to re-compute
	// the module and resource expansion.
	PlanTimeCheckResults *states.CheckResults

	// PlanTimeTimestamp should be populated during the plan phase by retrieving
	// the current UTC timestamp, and should be read from the plan file during
	// the apply phase.
	PlanTimeTimestamp time.Time

	MoveResults refactoring.MoveResults

	ProviderFunctionTracker ProviderFunctionMapping
}

func (c *Context) walk(ctx context.Context, graph *Graph, operation walkOperation, opts *graphWalkOpts) (*ContextGraphWalker, tfdiags.Diagnostics) {
	log.Printf("[DEBUG] Starting graph walk: %s", operation.String())

	walker := c.graphWalker(operation, opts)

	// Watch for a stop so we can call the provider Stop() API.
	watchStop, watchWait := c.watchStop(walker)

	// Walk the real graph, this will block until it completes
	diags := graph.Walk(ctx, walker)

	// Close the channel so the watcher stops, and wait for it to return.
	close(watchStop)
	<-watchWait

	return walker, diags
}

func (c *Context) graphWalker(operation walkOperation, opts *graphWalkOpts) *ContextGraphWalker {
	var state *states.SyncState
	var refreshState *states.SyncState
	var prevRunState *states.SyncState

	// NOTE: None of the SyncState objects must directly wrap opts.InputState,
	// because we use those to mutate the state object and opts.InputState
	// belongs to our caller and thus we must treat it as immutable.
	//
	// To account for that, most of our SyncState values created below end up
	// wrapping a _deep copy_ of opts.InputState instead.
	inputState := opts.InputState
	if inputState == nil {
		// Lots of callers use nil to represent the "empty" case where we've
		// not run Apply yet, so we tolerate that.
		inputState = states.NewState()
	}

	switch operation {
	case walkValidate:
		// validate should not use any state
		state = states.NewState().SyncWrapper()

		// validate currently uses the plan graph, so we have to populate the
		// refreshState and the prevRunState.
		refreshState = states.NewState().SyncWrapper()
		prevRunState = states.NewState().SyncWrapper()

	case walkPlan, walkPlanDestroy, walkImport:
		state = inputState.DeepCopy().SyncWrapper()
		refreshState = inputState.DeepCopy().SyncWrapper()
		prevRunState = inputState.DeepCopy().SyncWrapper()

		// For both of our new states we'll discard the previous run's
		// check results, since we can still refer to them from the
		// prevRunState object if we need to.
		state.DiscardCheckResults()
		refreshState.DiscardCheckResults()

	default:
		state = inputState.DeepCopy().SyncWrapper()
		// Only plan-like walks use refreshState and prevRunState

		// Discard the input state's check results, because we should create
		// a new set as a result of the graph walk.
		state.DiscardCheckResults()
	}

	changes := opts.Changes
	if changes == nil {
		// Several of our non-plan walks end up sharing codepaths with the
		// plan walk and thus expect to generate planned changes even though
		// we don't care about them. To avoid those crashing, we'll just
		// insert a placeholder changes object which'll get discarded
		// afterwards.
		changes = plans.NewChanges()
	}

	if opts.Config == nil {
		panic("Context.graphWalker call without Config")
	}

	checkState := checks.NewState(opts.Config)
	if opts.PlanTimeCheckResults != nil {
		// We'll re-report all of the same objects we determined during the
		// plan phase so that we can repeat the checks during the apply
		// phase to finalize them.
		for _, configElem := range opts.PlanTimeCheckResults.ConfigResults.Elems {
			if configElem.Value.ObjectAddrsKnown() {
				configAddr := configElem.Key
				checkState.ReportCheckableObjects(configAddr, configElem.Value.ObjectResults.Keys())
			}
		}
	}

	return &ContextGraphWalker{
		Context:                 c,
		State:                   state,
		Config:                  opts.Config,
		RefreshState:            refreshState,
		PrevRunState:            prevRunState,
		Changes:                 changes.SyncWrapper(),
		Checks:                  checkState,
		InstanceExpander:        instances.NewExpander(),
		MoveResults:             opts.MoveResults,
		ImportResolver:          NewImportResolver(),
		Operation:               operation,
		StopContext:             c.runContext,
		PlanTimestamp:           opts.PlanTimeTimestamp,
		Encryption:              c.encryption,
		ProviderFunctionTracker: opts.ProviderFunctionTracker,
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

// ValidateImportIDs is used during the validation phase to validate the import IDs of all import targets.
// This function works similarly to ExpandAndResolveImport, but it only validates the IDs of the import targets and does not modify the EvalContext.
// We only validate the IDs during the validation phase. Otherwise, we might cause a false positive,
// since we do not know if the user intends to use the '-generate-config-out' option to generate additional configuration, which would make invalid Addresses valid
func (ri *ImportResolver) ValidateImportIDs(ctx context.Context, importTarget *importing.ImportTarget, evalCtx EvalContext) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	// The import block expressions are declared within the root module.
	// We need to explicitly use the context with the path of the root module, so that all references will be
	// relative to the root module
	rootCtx := evalCtx.WithPath(addrs.RootModuleInstance)

	if importTarget.Config.ForEach != nil {
		const unknownsAllowed = true
		const tupleAllowed = true

		// The import target has a for_each attribute, so we need to expand it
		forEachVal, evalDiags := evaluateForEachExpressionValue(ctx, importTarget.Config.ForEach, rootCtx, unknownsAllowed, tupleAllowed, nil)
		diags = diags.Append(evalDiags)
		if diags.HasErrors() {
			return diags
		}

		// We are building an instances.RepetitionData based on each for_each key and val combination
		var repetitions []instances.RepetitionData

		if !forEachVal.IsKnown() {
			return diags
		}
		it := forEachVal.ElementIterator()
		for it.Next() {
			k, v := it.Element()
			repetitions = append(repetitions, instances.RepetitionData{
				EachKey:   k,
				EachValue: v,
			})
		}

		for _, keyData := range repetitions {
			evalDiags = validateImportIdExpression(importTarget.Config.ID, rootCtx, keyData)
			diags = diags.Append(evalDiags)
		}
	} else {
		// The import target is singular, no need to expand
		evalDiags := validateImportIdExpression(importTarget.Config.ID, rootCtx, EvalDataForNoInstanceKey)
		diags = diags.Append(evalDiags)
	}

	return diags
}

// ExpandAndResolveImport is responsible for two operations:
// 1. Expands the ImportTarget (originating from an import block) if it contains a 'for_each' attribute.
// 2. Goes over the expanded imports and resolves the ID and address, when we have the context necessary to resolve
// them. The resolved import target would be an EvaluatedConfigImportTarget.
// This function mutates the EvalContext's ImportResolver, adding the resolved import target.
// The function errors if we failed to evaluate the ID or the address.
func (ri *ImportResolver) ExpandAndResolveImport(ctx context.Context, importTarget *importing.ImportTarget, evalCtx EvalContext) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	// The import block expressions are declared within the root module.
	// We need to explicitly use the context with the path of the root module, so that all references will be
	// relative to the root module
	rootCtx := evalCtx.WithPath(addrs.RootModuleInstance)

	if importTarget.Config.ForEach != nil {
		const unknownsNotAllowed = false
		const tupleAllowed = true

		// The import target has a for_each attribute, so we need to expand it
		forEachVal, evalDiags := evaluateForEachExpressionValue(ctx, importTarget.Config.ForEach, rootCtx, unknownsNotAllowed, tupleAllowed, nil)
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
func (ri *ImportResolver) resolveImport(importTarget *importing.ImportTarget, ctx EvalContext, keyData instances.RepetitionData) tfdiags.Diagnostics {
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

// addCLIImportTarget adds a new import target originating from the CLI
// this is done to reuse Context.postExpansionImportValidation for CLI import validation
func (ri *ImportResolver) addCLIImportTarget(importTarget *importing.ImportTarget) {
	ri.mu.Lock()
	defer ri.mu.Unlock()
	importAddress := importTarget.CommandLineImportTarget.Addr
	ri.imports[importAddress.String()] = EvaluatedConfigImportTarget{
		// Since this import target originates from the CLI, and we have no config block for it
		// setting nil value to Config here to reuse Context.postExpansionImportValidation,
		// and there should be no possible paths to dereference this with a nil value during the import command
		Config: nil,
		Addr:   importAddress,
		ID:     importTarget.CommandLineImportTarget.ID,
	}
}
