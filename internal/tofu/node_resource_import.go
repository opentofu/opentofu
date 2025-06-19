// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type graphNodeImportState struct {
	Addr                addrs.AbsResourceInstance // Addr is the resource address to import into
	ID                  string                    // ID is the ID to import as
	ResolvedProvider    ResolvedProvider          // provider node address after resolution
	ResolvedProviderKey addrs.InstanceKey         // resolved from ResolvedProviderKeyExpr+ResolvedProviderKeyPath in method Execute

	Schema        *configschema.Block // Schema for processing the configuration body
	SchemaVersion uint64              // Schema version of "Schema", as decided by the provider
	Config        *configs.Resource   // Config is the resource in the config

	states []providers.ImportedResource
}

var (
	_ GraphNodeModulePath        = (*graphNodeImportState)(nil)
	_ GraphNodeExecutable        = (*graphNodeImportState)(nil)
	_ GraphNodeProviderConsumer  = (*graphNodeImportState)(nil)
	_ GraphNodeDynamicExpandable = (*graphNodeImportState)(nil)
)

func (n *graphNodeImportState) Name() string {
	return fmt.Sprintf("%s (import id %q)", n.Addr, n.ID)
}

// GraphNodeProviderConsumer
func (n *graphNodeImportState) ProvidedBy() RequestedProvider {
	// This has already been resolved by nodeExpandPlannableResource
	return RequestedProvider{
		ProviderConfig: n.ResolvedProvider.ProviderConfig,
		KeyExpression:  n.ResolvedProvider.KeyExpression,
		KeyModule:      n.ResolvedProvider.KeyModule,
		KeyResource:    n.ResolvedProvider.KeyResource,
		KeyExact:       n.ResolvedProvider.KeyExact,
	}
}

// GraphNodeProviderConsumer
func (n *graphNodeImportState) Provider() addrs.Provider {
	// This has already been resolved by nodeExpandPlannableResource
	return n.ResolvedProvider.ProviderConfig.Provider
}

// GraphNodeProviderConsumer
func (n *graphNodeImportState) SetProvider(resolved ResolvedProvider) {
	n.ResolvedProvider = resolved
}

// GraphNodeModuleInstance
func (n *graphNodeImportState) Path() addrs.ModuleInstance {
	return n.Addr.Module
}

// GraphNodeModulePath
func (n *graphNodeImportState) ModulePath() addrs.Module {
	return n.Addr.Module.Module()
}

// GraphNodeExecutable impl.
func (n *graphNodeImportState) Execute(ctx context.Context, evalCtx EvalContext, op walkOperation) (diags tfdiags.Diagnostics) {
	// Reset our states
	n.states = nil

	// FIXME, yuck: borrowing some logic that's currently only available for the abstract resource instance
	// node, even though graphNodeImportState doesn't actually embed that type for some reason.
	// Let's factor this logic out somewhere that's explicitly shareable.
	asAbsNode := &NodeAbstractResourceInstance{
		Addr: n.Addr,
		NodeAbstractResource: NodeAbstractResource{
			Addr:             n.Addr.ConfigResource(),
			Config:           n.Config,
			Schema:           n.Schema,
			SchemaVersion:    n.SchemaVersion,
			ResolvedProvider: n.ResolvedProvider,
		},
	}
	diags = diags.Append(asAbsNode.resolveProvider(ctx, evalCtx, true, states.NotDeposed))
	if diags.HasErrors() {
		return diags
	}
	n.ResolvedProviderKey = asAbsNode.ResolvedProviderKey
	log.Printf("[TRACE] graphNodeImportState: importing using %s", n.ResolvedProvider.ProviderConfig.InstanceString(n.ResolvedProviderKey))

	provider, _, err := getProvider(ctx, evalCtx, n.ResolvedProvider.ProviderConfig, n.ResolvedProviderKey)
	diags = diags.Append(err)
	if diags.HasErrors() {
		return diags
	}

	// import state
	absAddr := n.Addr.Resource.Absolute(evalCtx.Path())

	// Call pre-import hook
	diags = diags.Append(evalCtx.Hook(func(h Hook) (HookAction, error) {
		return h.PreImportState(absAddr, n.ID)
	}))
	if diags.HasErrors() {
		return diags
	}

	resp := provider.ImportResourceState(ctx, providers.ImportResourceStateRequest{
		TypeName: n.Addr.Resource.Resource.Type,
		ID:       n.ID,
	})
	diags = diags.Append(maybeImproveResourceInstanceDiagnostics(resp.Diagnostics, n.Addr))
	if diags.HasErrors() {
		return diags
	}

	imported := resp.ImportedResources
	for _, obj := range imported {
		log.Printf("[TRACE] graphNodeImportState: import %s %q produced instance object of type %s", absAddr.String(), n.ID, obj.TypeName)
	}
	n.states = imported

	// Call post-import hook
	diags = diags.Append(evalCtx.Hook(func(h Hook) (HookAction, error) {
		return h.PostImportState(absAddr, imported)
	}))
	return diags
}

// GraphNodeDynamicExpandable impl.
//
// We use DynamicExpand as a way to generate the subgraph of refreshes
// and state inserts we need to do for our import state. Since they're new
// resources they don't depend on anything else and refreshes are isolated
// so this is nearly a perfect use case for dynamic expand.
func (n *graphNodeImportState) DynamicExpand(evalCtx EvalContext) (*Graph, error) {
	var diags tfdiags.Diagnostics

	g := &Graph{Path: evalCtx.Path()}

	// nameCounter is used to de-dup names in the state.
	nameCounter := make(map[string]int)

	// Compile the list of addresses that we'll be inserting into the state.
	// We do this ahead of time so we can verify that we aren't importing
	// something that already exists.
	addrs := make([]addrs.AbsResourceInstance, len(n.states))
	for i, state := range n.states {
		addr := n.Addr
		if t := state.TypeName; t != "" {
			addr.Resource.Resource.Type = t
		}

		// Determine if we need to suffix the name to de-dup
		key := addr.String()
		count, ok := nameCounter[key]
		if ok {
			count++
			addr.Resource.Resource.Name += fmt.Sprintf("-%d", count)
		}
		nameCounter[key] = count

		// Add it to our list
		addrs[i] = addr
	}

	// Verify that all the addresses are clear
	state := evalCtx.State()
	for _, addr := range addrs {
		existing := state.ResourceInstance(addr)
		if existing != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Resource already managed by OpenTofu",
				fmt.Sprintf("OpenTofu is already managing a remote object for %s. To import to this address you must first remove the existing object from the state.", addr),
			))
			continue
		}
	}
	if diags.HasErrors() {
		// Bail out early, then.
		return nil, diags.Err()
	}

	// For each of the states, we add a node to handle the refresh/add to state.
	// "n.states" is populated by our own Execute with the result of
	// ImportState. Since DynamicExpand is always called after Execute, this is
	// safe.
	for i, state := range n.states {
		g.Add(&graphNodeImportStateSub{
			TargetAddr:          addrs[i],
			State:               state,
			ResolvedProvider:    n.ResolvedProvider,
			ResolvedProviderKey: n.ResolvedProviderKey,
			Schema:              n.Schema,
			SchemaVersion:       n.SchemaVersion,
			Config:              n.Config,
		})
	}

	addRootNodeToGraph(g)

	// Done!
	return g, diags.Err()
}

// graphNodeImportStateSub is the sub-node of graphNodeImportState
// and is part of the subgraph. This node is responsible for refreshing
// and adding a resource to the state once it is imported.
type graphNodeImportStateSub struct {
	TargetAddr          addrs.AbsResourceInstance
	State               providers.ImportedResource
	ResolvedProvider    ResolvedProvider
	ResolvedProviderKey addrs.InstanceKey // the dynamic instance ResolvedProvider

	Schema        *configschema.Block // Schema for processing the configuration body
	SchemaVersion uint64              // Schema version of "Schema", as decided by the provider
	Config        *configs.Resource   // Config is the resource in the config
}

var (
	_ GraphNodeModuleInstance = (*graphNodeImportStateSub)(nil)
	_ GraphNodeExecutable     = (*graphNodeImportStateSub)(nil)
)

func (n *graphNodeImportStateSub) Name() string {
	return fmt.Sprintf("import %s result", n.TargetAddr)
}

func (n *graphNodeImportStateSub) Path() addrs.ModuleInstance {
	return n.TargetAddr.Module
}

// GraphNodeExecutable impl.
func (n *graphNodeImportStateSub) Execute(ctx context.Context, evalCtx EvalContext, op walkOperation) (diags tfdiags.Diagnostics) {
	// If the Ephemeral type isn't set, then it is an error
	if n.State.TypeName == "" {
		diags = diags.Append(fmt.Errorf("import of %s didn't set type", n.TargetAddr.String()))
		return diags
	}

	state := n.State.AsInstanceObject()

	// Refresh
	riNode := &NodeAbstractResourceInstance{
		Addr: n.TargetAddr,
		NodeAbstractResource: NodeAbstractResource{
			ResolvedProvider: n.ResolvedProvider,
		},
		ResolvedProviderKey: n.ResolvedProviderKey,
	}
	state, refreshDiags := riNode.refresh(ctx, evalCtx, states.NotDeposed, state)
	diags = diags.Append(refreshDiags)
	if diags.HasErrors() {
		return diags
	}

	// Verify the existence of the imported resource
	if state.Value.IsNull() {
		var diags tfdiags.Diagnostics
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Cannot import non-existent remote object",
			fmt.Sprintf(
				"While attempting to import an existing object to %q, "+
					"the provider detected that no object exists with the given id. "+
					"Only pre-existing objects can be imported; check that the id "+
					"is correct and that it is associated with the provider's "+
					"configured region or endpoint, or use \"tofu apply\" to "+
					"create a new remote object for this resource.",
				n.TargetAddr,
			),
		))
		return diags
	}

	// Insert marks from configuration
	if n.Config != nil {
		// Since the import command allow import resource with incomplete configuration, we ignore diagnostics here
		valueWithConfigurationSchemaMarks, _, _ := evalCtx.EvaluateBlock(ctx, n.Config.Config, n.Schema, nil, EvalDataForNoInstanceKey)

		_, stateValueMarks := state.Value.UnmarkDeepWithPaths()
		_, valueWithConfigurationSchemaMarksPaths := valueWithConfigurationSchemaMarks.UnmarkDeepWithPaths()
		combined := combinePathValueMarks(stateValueMarks, valueWithConfigurationSchemaMarksPaths)
		state.Value = state.Value.MarkWithPaths(combined)
	}

	diags = diags.Append(riNode.writeResourceInstanceState(ctx, evalCtx, state, workingState))
	return diags
}
