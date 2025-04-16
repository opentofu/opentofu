// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"log"
	"path/filepath"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/genconfig"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// NodePlannableResourceInstance represents a _single_ resource
// instance that is plannable. This means this represents a single
// count index, for example.
type NodePlannableResourceInstance struct {
	*NodeAbstractResourceInstance
	ForceCreateBeforeDestroy bool

	// skipRefresh indicates that we should skip refreshing individual instances
	skipRefresh bool

	// skipPlanChanges indicates we should skip trying to plan change actions
	// for any instances.
	skipPlanChanges bool

	// forceReplace are resource instance addresses where the user wants to
	// force generating a replace action. This set isn't pre-filtered, so
	// it might contain addresses that have nothing to do with the resource
	// that this node represents, which the node itself must therefore ignore.
	forceReplace []addrs.AbsResourceInstance

	// replaceTriggeredBy stores references from replace_triggered_by which
	// triggered this instance to be replaced.
	replaceTriggeredBy []*addrs.Reference

	// importTarget, if populated, contains the information necessary to plan
	// an import of this resource.
	importTarget EvaluatedConfigImportTarget
}

// EvaluatedConfigImportTarget is a target that we need to import. It's created when an import target originated from
// an import block, after everything regarding the configuration has been evaluated.
// At this point, the import target is of a single resource instance
type EvaluatedConfigImportTarget struct {
	// Config is the original import block for this import. This might be null
	// if the import did not originate in config.
	Config *configs.Import

	// Addr is the actual address of the resource instance that we should import into. At this point, the address
	// should be fully evaluated
	Addr addrs.AbsResourceInstance

	// ID is the string ID of the resource to import. This is resource-instance specific.
	ID string
}

var (
	_ GraphNodeModuleInstance       = (*NodePlannableResourceInstance)(nil)
	_ GraphNodeReferenceable        = (*NodePlannableResourceInstance)(nil)
	_ GraphNodeReferencer           = (*NodePlannableResourceInstance)(nil)
	_ GraphNodeConfigResource       = (*NodePlannableResourceInstance)(nil)
	_ GraphNodeResourceInstance     = (*NodePlannableResourceInstance)(nil)
	_ GraphNodeAttachResourceConfig = (*NodePlannableResourceInstance)(nil)
	_ GraphNodeAttachResourceState  = (*NodePlannableResourceInstance)(nil)
	_ GraphNodeExecutable           = (*NodePlannableResourceInstance)(nil)
)

// GraphNodeEvalable
func (n *NodePlannableResourceInstance) Execute(ctx EvalContext, op walkOperation) tfdiags.Diagnostics {
	addr := n.ResourceInstanceAddr()

	diags := n.resolveProvider(ctx, true, states.NotDeposed)
	if diags.HasErrors() {
		return diags
	}

	// Eval info is different depending on what kind of resource this is
	switch addr.Resource.Resource.Mode {
	case addrs.ManagedResourceMode:
		return n.managedResourceExecute(ctx)
	case addrs.DataResourceMode:
		return n.dataResourceExecute(ctx)
	default:
		panic(fmt.Errorf("unsupported resource mode %s", n.Config.Mode))
	}
}

func (n *NodePlannableResourceInstance) dataResourceExecute(ctx EvalContext) (diags tfdiags.Diagnostics) {
	config := n.Config
	addr := n.ResourceInstanceAddr()

	var change *plans.ResourceInstanceChange

	_, providerSchema, err := getProvider(ctx, n.ResolvedProvider.ProviderConfig, n.ResolvedProviderKey)
	diags = diags.Append(err)
	if diags.HasErrors() {
		return diags
	}

	diags = diags.Append(validateSelfRef(addr.Resource, config.Config, providerSchema))
	if diags.HasErrors() {
		return diags
	}

	checkRuleSeverity := tfdiags.Error
	if n.skipPlanChanges || n.preDestroyRefresh {
		checkRuleSeverity = tfdiags.Warning
	}

	change, state, repeatData, planDiags := n.planDataSource(ctx, checkRuleSeverity, n.skipPlanChanges)
	diags = diags.Append(planDiags)
	if diags.HasErrors() {
		return diags
	}

	// write the data source into both the refresh state and the
	// working state
	diags = diags.Append(n.writeResourceInstanceState(ctx, state, refreshState))
	if diags.HasErrors() {
		return diags
	}
	diags = diags.Append(n.writeResourceInstanceState(ctx, state, workingState))
	if diags.HasErrors() {
		return diags
	}

	diags = diags.Append(n.writeChange(ctx, change, ""))

	// Post-conditions might block further progress. We intentionally do this
	// _after_ writing the state/diff because we want to check against
	// the result of the operation, and to fail on future operations
	// until the user makes the condition succeed.
	checkDiags := evalCheckRules(
		addrs.ResourcePostcondition,
		n.Config.Postconditions,
		ctx, addr, repeatData,
		checkRuleSeverity,
	)
	diags = diags.Append(checkDiags)

	return diags
}

func (n *NodePlannableResourceInstance) managedResourceExecute(ctx EvalContext) (diags tfdiags.Diagnostics) {
	config := n.Config
	addr := n.ResourceInstanceAddr()

	var instanceRefreshState *states.ResourceInstanceObject

	checkRuleSeverity := tfdiags.Error
	if n.skipPlanChanges || n.preDestroyRefresh {
		checkRuleSeverity = tfdiags.Warning
	}

	provider, providerSchema, err := getProvider(ctx, n.ResolvedProvider.ProviderConfig, n.ResolvedProviderKey)
	diags = diags.Append(err)
	if diags.HasErrors() {
		return diags
	}

	if config != nil {
		diags = diags.Append(validateSelfRef(addr.Resource, config.Config, providerSchema))
		if diags.HasErrors() {
			return diags
		}
	}

	importing := n.shouldImport(ctx)

	if importing && n.Config == nil && len(n.generateConfigPath) == 0 {
		// Then the user wrote an import target to a target that didn't exist.
		if n.Addr.Module.IsRoot() {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Import block target does not exist",
				Detail:   "The target for the given import block does not exist. If you wish to automatically generate config for this resource, use the -generate-config-out option within tofu plan. Otherwise, make sure the target resource exists within your configuration. For example:\n\n  tofu plan -generate-config-out=generated.tf",
				Subject:  n.importTarget.Config.DeclRange.Ptr(),
			})
		} else {
			// You can't generate config for a resource that is inside a
			// module, so we will present a different error message for
			// this case.
			diags = diags.Append(importResourceWithoutConfigDiags(n.Addr.String(), n.importTarget.Config))
		}
		return diags
	}

	// If the resource is to be imported, we now ask the provider for an Import
	// and a Refresh, and save the resulting state to instanceRefreshState.
	if importing {
		instanceRefreshState, diags = n.importState(ctx, addr, n.importTarget.ID, provider, providerSchema)
	} else {
		var readDiags tfdiags.Diagnostics
		instanceRefreshState, readDiags = n.readResourceInstanceState(ctx, addr)
		diags = diags.Append(readDiags)
		if diags.HasErrors() {
			return diags
		}
	}

	// We'll save a snapshot of what we just read from the state into the
	// prevRunState before we do anything else, since this will capture the
	// result of any schema upgrading that readResourceInstanceState just did,
	// but not include any out-of-band changes we might detect in in the
	// refresh step below.
	diags = diags.Append(n.writeResourceInstanceState(ctx, instanceRefreshState, prevRunState))
	if diags.HasErrors() {
		return diags
	}
	// Also the refreshState, because that should still reflect schema upgrades
	// even if it doesn't reflect upstream changes.
	diags = diags.Append(n.writeResourceInstanceState(ctx, instanceRefreshState, refreshState))
	if diags.HasErrors() {
		return diags
	}

	// In 0.13 we could be refreshing a resource with no config.
	// We should be operating on managed resource, but check here to be certain
	if n.Config == nil || n.Config.Managed == nil {
		log.Printf("[WARN] managedResourceExecute: no Managed config value found in instance state for %q", n.Addr)
	} else {
		if instanceRefreshState != nil {
			prevCreateBeforeDestroy := instanceRefreshState.CreateBeforeDestroy

			// This change is usually written to the refreshState and then
			// updated value used for further graph execution. However, with
			// "refresh=false", refreshState is not being written and then
			// some resources with updated configuration could be detached
			// due to missaligned create_before_destroy in different graph nodes.
			instanceRefreshState.CreateBeforeDestroy = n.Config.Managed.CreateBeforeDestroy || n.ForceCreateBeforeDestroy

			if prevCreateBeforeDestroy != instanceRefreshState.CreateBeforeDestroy && n.skipRefresh {
				diags = diags.Append(n.writeResourceInstanceState(ctx, instanceRefreshState, refreshState))
				if diags.HasErrors() {
					return diags
				}
			}
		}
	}

	// Refresh, maybe
	// The import process handles its own refresh
	if !n.skipRefresh && !importing {
		s, refreshDiags := n.refresh(ctx, states.NotDeposed, instanceRefreshState)
		diags = diags.Append(refreshDiags)
		if diags.HasErrors() {
			return diags
		}

		instanceRefreshState = s

		if instanceRefreshState != nil {
			// When refreshing we start by merging the stored dependencies and
			// the configured dependencies. The configured dependencies will be
			// stored to state once the changes are applied. If the plan
			// results in no changes, we will re-write these dependencies
			// below.
			instanceRefreshState.Dependencies = mergeDeps(n.Dependencies, instanceRefreshState.Dependencies)
		}

		diags = diags.Append(n.writeResourceInstanceState(ctx, instanceRefreshState, refreshState))
		if diags.HasErrors() {
			return diags
		}
	}

	// Plan the instance, unless we're in the refresh-only mode
	if !n.skipPlanChanges {

		// add this instance to n.forceReplace if replacement is triggered by
		// another change
		repData := instances.RepetitionData{}
		switch k := addr.Resource.Key.(type) {
		case addrs.IntKey:
			repData.CountIndex = k.Value()
		case addrs.StringKey:
			repData.EachKey = k.Value()
			repData.EachValue = cty.DynamicVal
		}

		diags = diags.Append(n.replaceTriggered(ctx, repData))
		if diags.HasErrors() {
			return diags
		}

		change, instancePlanState, repeatData, planDiags := n.plan(
			ctx, nil, instanceRefreshState, n.ForceCreateBeforeDestroy, n.forceReplace,
		)
		diags = diags.Append(planDiags)
		if diags.HasErrors() {
			// If we are importing and generating a configuration, we need to
			// ensure the change is written out so the configuration can be
			// captured.
			if len(n.generateConfigPath) > 0 {
				// Update our return plan
				change := &plans.ResourceInstanceChange{
					Addr:         n.Addr,
					PrevRunAddr:  n.prevRunAddr(ctx),
					ProviderAddr: n.ResolvedProvider.ProviderConfig,
					Change: plans.Change{
						// we only need a placeholder, so this will be a NoOp
						Action:          plans.NoOp,
						Before:          instanceRefreshState.Value,
						After:           instanceRefreshState.Value,
						GeneratedConfig: n.generatedConfigHCL,
					},
				}
				diags = diags.Append(n.writeChange(ctx, change, ""))
			}

			return diags
		}

		if importing {
			change.Importing = &plans.Importing{ID: n.importTarget.ID}
		}

		// FIXME: here we update the change to reflect the reason for
		// replacement, but we still overload forceReplace to get the correct
		// change planned.
		if len(n.replaceTriggeredBy) > 0 {
			change.ActionReason = plans.ResourceInstanceReplaceByTriggers
		}

		// FIXME: it is currently important that we write resource changes to
		// the plan (n.writeChange) before we write the corresponding state
		// (n.writeResourceInstanceState).
		//
		// This is because the planned resource state will normally have the
		// status of states.ObjectPlanned, which causes later logic to refer to
		// the contents of the plan to retrieve the resource data. Because
		// there is no shared lock between these two data structures, reversing
		// the order of these writes will cause a brief window of inconsistency
		// which can lead to a failed safety check.
		//
		// Future work should adjust these APIs such that it is impossible to
		// update these two data structures incorrectly through any objects
		// reachable via the tofu.EvalContext API.
		diags = diags.Append(n.writeChange(ctx, change, ""))
		if diags.HasErrors() {
			return diags
		}
		diags = diags.Append(n.checkPreventDestroy(ctx, change))
		if diags.HasErrors() {
			return diags
		}

		diags = diags.Append(n.writeResourceInstanceState(ctx, instancePlanState, workingState))
		if diags.HasErrors() {
			return diags
		}

		// If this plan resulted in a NoOp, then apply won't have a chance to make
		// any changes to the stored dependencies. Since this is a NoOp we know
		// that the stored dependencies will have no effect during apply, and we can
		// write them out now.
		if change.Action == plans.NoOp && !depsEqual(instanceRefreshState.Dependencies, n.Dependencies) {
			// the refresh state will be the final state for this resource, so
			// finalize the dependencies here if they need to be updated.
			instanceRefreshState.Dependencies = n.Dependencies
			diags = diags.Append(n.writeResourceInstanceState(ctx, instanceRefreshState, refreshState))
			if diags.HasErrors() {
				return diags
			}
		}

		// Post-conditions might block completion. We intentionally do this
		// _after_ writing the state/diff because we want to check against
		// the result of the operation, and to fail on future operations
		// until the user makes the condition succeed.
		// (Note that some preconditions will end up being skipped during
		// planning, because their conditions depend on values not yet known.)
		checkDiags := evalCheckRules(
			addrs.ResourcePostcondition,
			n.Config.Postconditions,
			ctx, n.ResourceInstanceAddr(), repeatData,
			checkRuleSeverity,
		)
		diags = diags.Append(checkDiags)
	} else {
		// In refresh-only mode we need to evaluate the for-each expression in
		// order to supply the value to the pre- and post-condition check
		// blocks. This has the unfortunate edge case of a refresh-only plan
		// executing with a for-each map which has the same keys but different
		// values, which could result in a post-condition check relying on that
		// value being inaccurate. Unless we decide to store the value of the
		// for-each expression in state, this is unavoidable.
		forEach, _ := evaluateForEachExpression(n.Config.ForEach, ctx, n.ResourceAddr())
		repeatData := EvalDataForInstanceKey(n.ResourceInstanceAddr().Resource.Key, forEach)

		checkDiags := evalCheckRules(
			addrs.ResourcePrecondition,
			n.Config.Preconditions,
			ctx, addr, repeatData,
			checkRuleSeverity,
		)
		diags = diags.Append(checkDiags)

		// Even if we don't plan changes, we do still need to at least update
		// the working state to reflect the refresh result. If not, then e.g.
		// any output values referring to this will not react to the drift.
		// (Even if we didn't actually refresh above, this will still save
		// the result of any schema upgrading we did in readResourceInstanceState.)
		diags = diags.Append(n.writeResourceInstanceState(ctx, instanceRefreshState, workingState))
		if diags.HasErrors() {
			return diags
		}

		// Here we also evaluate post-conditions after updating the working
		// state, because we want to check against the result of the refresh.
		// Unlike in normal planning mode, these checks are still evaluated
		// even if pre-conditions generated diagnostics, because we have no
		// planned changes to block.
		checkDiags = evalCheckRules(
			addrs.ResourcePostcondition,
			n.Config.Postconditions,
			ctx, addr, repeatData,
			checkRuleSeverity,
		)
		diags = diags.Append(checkDiags)
	}

	return diags
}

// replaceTriggered checks if this instance needs to be replace due to a change
// in a replace_triggered_by reference. If replacement is required, the
// instance address is added to forceReplace
func (n *NodePlannableResourceInstance) replaceTriggered(ctx EvalContext, repData instances.RepetitionData) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if n.Config == nil {
		return diags
	}

	for _, expr := range n.Config.TriggersReplacement {
		ref, replace, evalDiags := ctx.EvaluateReplaceTriggeredBy(expr, repData)
		diags = diags.Append(evalDiags)
		if diags.HasErrors() {
			continue
		}

		if replace {
			// FIXME: forceReplace accomplishes the same goal, however we may
			// want to communicate more information about which resource
			// triggered the replacement in the plan.
			// Rather than further complicating the plan method with more
			// options, we can refactor both of these features later.
			n.forceReplace = append(n.forceReplace, n.Addr)
			log.Printf("[DEBUG] ReplaceTriggeredBy forcing replacement of %s due to change in %s", n.Addr, ref.DisplayString())

			n.replaceTriggeredBy = append(n.replaceTriggeredBy, ref)
			break
		}
	}

	return diags
}

func (n *NodePlannableResourceInstance) importState(ctx EvalContext, addr addrs.AbsResourceInstance, importId string, provider providers.Interface, providerSchema providers.ProviderSchema) (*states.ResourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	absAddr := addr.Resource.Absolute(ctx.Path())

	diags = diags.Append(ctx.Hook(func(h Hook) (HookAction, error) {
		return h.PrePlanImport(absAddr, importId)
	}))
	if diags.HasErrors() {
		return nil, diags
	}

	resp := provider.ImportResourceState(providers.ImportResourceStateRequest{
		TypeName: addr.Resource.Resource.Type,
		ID:       importId,
	})
	diags = diags.Append(resp.Diagnostics)
	if diags.HasErrors() {
		return nil, diags
	}

	imported := resp.ImportedResources

	if len(imported) == 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Import returned no resources",
			fmt.Sprintf("While attempting to import with ID %s, the provider"+
				"returned no instance states.",
				importId,
			),
		))
		return nil, diags
	}
	for _, obj := range imported {
		log.Printf("[TRACE] graphNodeImportState: import %s %q produced instance object of type %s", absAddr.String(), importId, obj.TypeName)
	}
	if len(imported) > 1 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Multiple import states not supported",
			fmt.Sprintf("While attempting to import with ID %s, the provider "+
				"returned multiple resource instance states. This "+
				"is not currently supported.",
				importId,
			),
		))
		return nil, diags
	}

	// call post-import hook
	diags = diags.Append(ctx.Hook(func(h Hook) (HookAction, error) {
		return h.PostPlanImport(absAddr, imported)
	}))

	if imported[0].TypeName == "" {
		diags = diags.Append(fmt.Errorf("import of %s didn't set type", n.Addr.String()))
		return nil, diags
	}

	importedState := imported[0].AsInstanceObject()

	if importedState.Value.IsNull() {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Import returned null resource",
			fmt.Sprintf("While attempting to import with ID %s, the provider"+
				"returned an instance with no state.",
				n.importTarget.ID,
			),
		))
	}

	// refresh
	riNode := &NodeAbstractResourceInstance{
		Addr: n.Addr,
		NodeAbstractResource: NodeAbstractResource{
			ResolvedProvider: n.ResolvedProvider,
		},
		ResolvedProviderKey: n.ResolvedProviderKey,
	}
	instanceRefreshState, refreshDiags := riNode.refresh(ctx, states.NotDeposed, importedState)
	diags = diags.Append(refreshDiags)
	if diags.HasErrors() {
		return instanceRefreshState, diags
	}

	// verify the existence of the imported resource
	if instanceRefreshState.Value.IsNull() {
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
				n.Addr,
			),
		))
		return instanceRefreshState, diags
	}

	// Insert marks from configuration
	if n.Config != nil {
		keyData := EvalDataForNoInstanceKey

		switch {
		case n.Config.Count != nil:
			keyData = InstanceKeyEvalData{
				CountIndex: cty.UnknownVal(cty.Number),
			}
		case n.Config.ForEach != nil:
			keyData = InstanceKeyEvalData{
				EachKey:   cty.UnknownVal(cty.String),
				EachValue: cty.UnknownVal(cty.DynamicPseudoType),
			}
		}

		valueWithConfigurationSchemaMarks, _, configDiags := ctx.EvaluateBlock(n.Config.Config, n.Schema, nil, keyData)
		diags = diags.Append(configDiags)
		if configDiags.HasErrors() {
			return instanceRefreshState, diags
		}

		_, marks := instanceRefreshState.Value.UnmarkDeepWithPaths()
		_, configSchemaMarks := valueWithConfigurationSchemaMarks.UnmarkDeepWithPaths()
		merged := combinePathValueMarks(marks, configSchemaMarks)

		instanceRefreshState.Value = instanceRefreshState.Value.MarkWithPaths(merged)
	}

	// If we're importing and generating config, generate it now.
	if len(n.generateConfigPath) > 0 {
		if n.Config != nil {
			return instanceRefreshState, diags.Append(fmt.Errorf("tried to generate config for %s, but it already exists", n.Addr))
		}

		schema, _ := providerSchema.SchemaForResourceAddr(n.Addr.Resource.Resource)
		if schema == nil {
			// Should be caught during validation, so we don't bother with a pretty error here
			diags = diags.Append(fmt.Errorf("provider does not support resource type for %q", n.Addr))
			return instanceRefreshState, diags
		}

		// Generate the HCL string first, then parse the HCL body from it.
		// First we generate the contents of the resource block for use within
		// the planning node. Then we wrap it in an enclosing resource block to
		// pass into the plan for rendering.
		generatedHCLAttributes, generatedDiags := n.generateHCLStringAttributes(n.Addr, instanceRefreshState, schema)
		diags = diags.Append(generatedDiags)

		n.generatedConfigHCL = genconfig.WrapResourceContents(n.Addr, generatedHCLAttributes)

		// parse the "file" as HCL to get the hcl.Body
		synthHCLFile, hclDiags := hclsyntax.ParseConfig([]byte(generatedHCLAttributes), filepath.Base(n.generateConfigPath), hcl.Pos{Byte: 0, Line: 1, Column: 1})
		diags = diags.Append(hclDiags)
		if hclDiags.HasErrors() {
			return instanceRefreshState, diags
		}

		// We have to do a kind of mini parsing of the content here to correctly
		// mark attributes like 'provider' as hidden. We only care about the
		// resulting content, so it's remain that gets passed into the resource
		// as the config.
		_, remain, resourceDiags := synthHCLFile.Body.PartialContent(configs.ResourceBlockSchema)
		diags = diags.Append(resourceDiags)
		if resourceDiags.HasErrors() {
			return instanceRefreshState, diags
		}

		n.Config = &configs.Resource{
			Mode:     addrs.ManagedResourceMode,
			Type:     n.Addr.Resource.Resource.Type,
			Name:     n.Addr.Resource.Resource.Name,
			Config:   remain,
			Managed:  &configs.ManagedResource{},
			Provider: n.ResolvedProvider.ProviderConfig.Provider,
		}
	}

	diags = diags.Append(riNode.writeResourceInstanceState(ctx, instanceRefreshState, refreshState))
	return instanceRefreshState, diags
}

func (n *NodePlannableResourceInstance) shouldImport(ctx EvalContext) bool {
	if n.importTarget.ID == "" {
		return false
	}

	// If the import target already has a state - we should not attempt to import it, but instead run a normal plan
	// for it
	state := ctx.State()
	return state.ResourceInstance(n.ResourceInstanceAddr()) == nil
}

// generateHCLStringAttributes produces a string in HCL format for the given
// resource state and schema without the surrounding block.
func (n *NodePlannableResourceInstance) generateHCLStringAttributes(addr addrs.AbsResourceInstance, state *states.ResourceInstanceObject, schema *configschema.Block) (string, tfdiags.Diagnostics) {
	filteredSchema := schema.Filter(
		configschema.FilterOr(
			configschema.FilterReadOnlyAttribute,
			configschema.FilterDeprecatedAttribute,

			// The legacy SDK adds an Optional+Computed "id" attribute to the
			// resource schema even if not defined in provider code.
			// During validation, however, the presence of an extraneous "id"
			// attribute in config will cause an error.
			// Remove this attribute so we do not generate an "id" attribute
			// where there is a risk that it is not in the real resource schema.
			//
			// TRADEOFF: Resources in which there actually is an
			// Optional+Computed "id" attribute in the schema will have that
			// attribute missing from generated config.
			configschema.FilterHelperSchemaIdAttribute,
		),
		configschema.FilterDeprecatedBlock,
	)

	providerAddr := addrs.LocalProviderConfig{
		LocalName: n.ResolvedProvider.ProviderConfig.Provider.Type,
		Alias:     n.ResolvedProvider.ProviderConfig.Alias,
	}

	return genconfig.GenerateResourceContents(addr, filteredSchema, providerAddr, state.Value)
}

// mergeDeps returns the union of 2 sets of dependencies
func mergeDeps(a, b []addrs.ConfigResource) []addrs.ConfigResource {
	switch {
	case len(a) == 0:
		return b
	case len(b) == 0:
		return a
	}

	set := make(map[string]addrs.ConfigResource)

	for _, dep := range a {
		set[dep.String()] = dep
	}

	for _, dep := range b {
		set[dep.String()] = dep
	}

	newDeps := make([]addrs.ConfigResource, 0, len(set))
	for _, dep := range set {
		newDeps = append(newDeps, dep)
	}

	return newDeps
}

func depsEqual(a, b []addrs.ConfigResource) bool {
	if len(a) != len(b) {
		return false
	}

	// Because we need to sort the deps to compare equality, make shallow
	// copies to prevent concurrently modifying the array values on
	// dependencies shared between expanded instances.
	copyA, copyB := make([]addrs.ConfigResource, len(a)), make([]addrs.ConfigResource, len(b))
	copy(copyA, a)
	copy(copyB, b)
	a, b = copyA, copyB

	less := func(s []addrs.ConfigResource) func(i, j int) bool {
		return func(i, j int) bool {
			return s[i].String() < s[j].String()
		}
	}

	sort.Slice(a, less(a))
	sort.Slice(b, less(b))

	for i := range a {
		if !a[i].Equal(b[i]) {
			return false
		}
	}
	return true
}
