// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package graph

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/refactoring"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu/contract"
)

func (c *Context) Plan(ctx context.Context, config *configs.Config, prevRunState *states.State, moveStmts []refactoring.MoveStatement, moveResults refactoring.MoveResults, opts *contract.PlanOpts) (*plans.Plan, tfdiags.Diagnostics) {
	defer c.acquireRun("plan")()

	var diags tfdiags.Diagnostics

	providerFunctionTracker := make(ProviderFunctionMapping)

	graph, walkOp, moreDiags := c.planGraph(ctx, config, prevRunState, opts, providerFunctionTracker)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	timestamp := time.Now().UTC()

	// If we get here then we should definitely have a non-nil "graph", which
	// we can now walk.
	changes := plans.NewChanges()
	walker, walkDiags := c.walk(ctx, graph, walkOp, &graphWalkOpts{
		Config:                  config,
		InputState:              prevRunState,
		Changes:                 changes,
		MoveResults:             moveResults,
		PlanTimeTimestamp:       timestamp,
		ProviderFunctionTracker: providerFunctionTracker,
	})
	diags = diags.Append(walker.NonFatalDiagnostics)
	diags = diags.Append(walkDiags)

	allInsts := walker.InstanceExpander.AllInstances()

	importValidateDiags := c.postExpansionImportValidation(walker.ImportResolver, allInsts)
	if importValidateDiags.HasErrors() {
		return nil, importValidateDiags
	}

	moveValidateDiags := c.postPlanValidateMoves(config, moveStmts, allInsts)
	if moveValidateDiags.HasErrors() {
		// If any of the move statements are invalid then those errors take
		// precedence over any other errors because an incomplete move graph
		// is quite likely to be the _cause_ of various errors. This oddity
		// comes from the fact that we need to apply the moves before we
		// actually validate them, because validation depends on the result
		// of first trying to plan.
		return nil, moveValidateDiags
	}
	diags = diags.Append(moveValidateDiags) // might just contain warnings

	if moveResults.Blocked.Len() > 0 && !diags.HasErrors() {
		// If we had blocked moves and we're not going to be returning errors
		// then we'll report the blockers as a warning. We do this only in the
		// absence of errors because invalid move statements might well be
		// the root cause of the blockers, and so better to give an actionable
		// error message than a less-actionable warning.
		diags = diags.Append(blockedMovesWarningDiag(moveResults))
	}

	// If we reach this point with error diagnostics then "changes" is a
	// representation of the subset of changes we were able to plan before
	// we encountered errors, which we'll return as part of a non-nil plan
	// so that e.g. the UI can show what was planned so far in case that extra
	// context helps the user to understand the error messages we're returning.
	prevRunState = walker.PrevRunState.Close()

	// The refreshed state may have data resource objects which were deferred
	// to apply and cannot be serialized.
	walker.RefreshState.RemovePlannedResourceInstanceObjects()
	priorState := walker.RefreshState.Close()

	driftedResources, driftDiags := c.driftedResources(ctx, config, prevRunState, priorState, moveResults)
	diags = diags.Append(driftDiags)

	plan := &plans.Plan{
		UIMode:             opts.Mode,
		Changes:            changes,
		DriftedResources:   driftedResources,
		PrevRunState:       prevRunState,
		PriorState:         priorState,
		PlannedState:       walker.State.Close(),
		ExternalReferences: opts.ExternalReferences,
		Checks:             states.NewCheckResults(walker.Checks),
		Timestamp:          timestamp,

		// Other fields get populated by Context.Plan after we return
	}

	if !diags.HasErrors() {
		diags = diags.Append(c.checkApplyGraph(ctx, plan, config))
	}

	return plan, diags
}

// checkApplyGraph builds the apply graph out of the current plan to
// check for any errors that may arise once the planned changes are added to
// the graph. This allows tofu to report errors (mostly cycles) during
// plan that would otherwise only crop up during apply
func (c *Context) checkApplyGraph(ctx context.Context, plan *plans.Plan, config *configs.Config) tfdiags.Diagnostics {
	if plan.Changes.Empty() {
		log.Println("[DEBUG] no planned changes, skipping apply graph check")
		return nil
	}
	log.Println("[DEBUG] building apply graph to check for errors")
	_, _, diags := c.applyGraph(ctx, plan, config, make(ProviderFunctionMapping), nil)
	return diags
}

// driftedResources is a best-effort attempt to compare the current and prior
// state. If we cannot decode the prior state for some reason, this should only
// return warnings to help the user correlate any missing resources in the
// report. This is known to happen when targeting a subset of resources,
// because the excluded instances will have been removed from the plan and
// not upgraded.
func (c *Context) driftedResources(ctx context.Context, config *configs.Config, oldState, newState *states.State, moves refactoring.MoveResults) ([]*plans.ResourceInstanceChangeSrc, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	if newState.ManagedResourcesEqual(oldState) && moves.Changes.Len() == 0 {
		// Nothing to do, because we only detect and report drift for managed
		// resource instances.
		return nil, diags
	}
	return nil, diags
	/* TODO cam72cam

	schemas, schemaDiags := c.Schemas(ctx, config, newState)
	diags = diags.Append(schemaDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	var drs []*plans.ResourceInstanceChangeSrc

	for _, ms := range oldState.Modules {
		for _, rs := range ms.Resources {
			if rs.Addr.Resource.Mode != addrs.ManagedResourceMode {
				// Drift reporting is only for managed resources
				continue
			}

			provider := rs.ProviderConfig.Provider
			for key, oldIS := range rs.Instances {
				if oldIS.Current == nil {
					// Not interested in instances that only have deposed objects
					continue
				}
				addr := rs.Addr.Instance(key)

				// Previous run address defaults to the current address, but
				// can differ if the resource moved before refreshing
				prevRunAddr := addr
				if move, ok := moves.Changes.GetOk(addr); ok {
					prevRunAddr = move.From
				}

				if isResourceMovedToDifferentType(addr, prevRunAddr) {
					// We don't report drift in case of resource type change
					continue
				}

				newIS := newState.ResourceInstance(addr)
				schema, _ := schemas.ResourceTypeConfig(
					provider,
					addr.Resource.Resource.Mode,
					addr.Resource.Resource.Type,
				)
				if schema == nil {
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Warning,
						"Missing resource schema from provider",
						fmt.Sprintf("No resource schema found for %s when decoding prior state", addr.Resource.Resource.Type),
					))
					continue
				}
				ty := schema.ImpliedType()

				oldObj, err := oldIS.Current.Decode(ty)
				if err != nil {
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Warning,
						"Failed to decode resource from state",
						fmt.Sprintf("Error decoding %q from prior state: %s", addr.String(), err),
					))
					continue
				}

				var newObj *states.ResourceInstanceObject
				if newIS != nil && newIS.Current != nil {
					newObj, err = newIS.Current.Decode(ty)
					if err != nil {
						diags = diags.Append(tfdiags.Sourceless(
							tfdiags.Warning,
							"Failed to decode resource from state",
							fmt.Sprintf("Error decoding %q from prior state: %s", addr.String(), err),
						))
						continue
					}
				}

				var oldVal, newVal cty.Value
				oldVal = oldObj.Value
				if newObj != nil {
					newVal = newObj.Value
				} else {
					newVal = cty.NullVal(ty)
				}

				if oldVal.RawEquals(newVal) && addr.Equal(prevRunAddr) {
					// No drift if the two values are semantically equivalent
					// and no move has happened
					continue
				}

				// We can detect three types of changes after refreshing state,
				// only two of which are easily understood as "drift":
				//
				// - Resources which were deleted outside OpenTofu;
				// - Resources where the object value has changed outside OpenTofu;
				// - Resources which have been moved without other changes.
				//
				// All of these are returned as drift, to allow refresh-only plans
				// to present a full set of changes which will be applied.
				var action plans.Action
				switch {
				case newVal.IsNull():
					action = plans.Delete
				case !oldVal.RawEquals(newVal):
					action = plans.Update
				default:
					action = plans.NoOp
				}

				change := &plans.ResourceInstanceChange{
					Addr:         addr,
					PrevRunAddr:  prevRunAddr,
					ProviderAddr: rs.ProviderConfig,
					Change: plans.Change{
						Action: action,
						Before: oldVal,
						After:  newVal,
					},
				}

				changeSrc, err := change.Encode(ty)
				if err != nil {
					diags = diags.Append(err)
					return nil, diags
				}

				drs = append(drs, changeSrc)
			}
		}
	}

	return drs, diags
	*/
}

func blockedMovesWarningDiag(results refactoring.MoveResults) tfdiags.Diagnostic {
	if results.Blocked.Len() < 1 {
		// Caller should check first
		panic("request to render blocked moves warning without any blocked moves")
	}

	var itemsBuf bytes.Buffer
	for _, blocked := range results.Blocked.Values() {
		fmt.Fprintf(&itemsBuf, "\n  - %s could not move to %s", blocked.Actual, blocked.Wanted)
	}

	return tfdiags.Sourceless(
		tfdiags.Warning,
		"Unresolved resource instance address changes",
		fmt.Sprintf(
			"OpenTofu tried to adjust resource instance addresses in the prior state based on change information recorded in the configuration, but some adjustments did not succeed due to existing objects already at the intended addresses:%s\n\nOpenTofu has planned to destroy these objects. If OpenTofu's proposed changes aren't appropriate, you must first resolve the conflicts using the \"tofu state\" subcommands and then create a new plan.",
			itemsBuf.String(),
		),
	)
}

func (c *Context) planGraph(ctx context.Context, config *configs.Config, prevRunState *states.State, opts *contract.PlanOpts, providerFunctionTracker ProviderFunctionMapping) (*Graph, walkOperation, tfdiags.Diagnostics) {
	switch mode := opts.Mode; mode {
	case plans.NormalMode:
		graph, diags := (&PlanGraphBuilder{
			Config:                  config,
			State:                   prevRunState,
			RootVariableValues:      opts.SetVariables,
			Plugins:                 c.plugins,
			Targets:                 opts.Targets,
			Excludes:                opts.Excludes,
			ForceReplace:            opts.ForceReplace,
			skipRefresh:             opts.SkipRefresh,
			preDestroyRefresh:       opts.PreDestroyRefresh,
			Operation:               walkPlan,
			ExternalReferences:      opts.ExternalReferences,
			ImportTargets:           opts.ImportTargets,
			GenerateConfigPath:      opts.GenerateConfigPath,
			RemoveStatements:        opts.RemoveStatements,
			ProviderFunctionTracker: providerFunctionTracker,
		}).Build(ctx, addrs.RootModuleInstance)
		return graph, walkPlan, diags
	case plans.RefreshOnlyMode:
		graph, diags := (&PlanGraphBuilder{
			Config:                  config,
			State:                   prevRunState,
			RootVariableValues:      opts.SetVariables,
			Plugins:                 c.plugins,
			Targets:                 opts.Targets,
			Excludes:                opts.Excludes,
			skipRefresh:             opts.SkipRefresh,
			skipPlanChanges:         true, // this activates "refresh only" mode.
			Operation:               walkPlan,
			ExternalReferences:      opts.ExternalReferences,
			ProviderFunctionTracker: providerFunctionTracker,
		}).Build(ctx, addrs.RootModuleInstance)
		return graph, walkPlan, diags
	case plans.DestroyMode:
		graph, diags := (&PlanGraphBuilder{
			Config:                  config,
			State:                   prevRunState,
			RootVariableValues:      opts.SetVariables,
			Plugins:                 c.plugins,
			Targets:                 opts.Targets,
			Excludes:                opts.Excludes,
			skipRefresh:             opts.SkipRefresh,
			Operation:               walkPlanDestroy,
			ProviderFunctionTracker: providerFunctionTracker,
		}).Build(ctx, addrs.RootModuleInstance)
		return graph, walkPlanDestroy, diags
	default:
		// The above should cover all plans.Mode values
		panic(fmt.Sprintf("unsupported plan mode %s", mode))
	}
}

// PlanGraphForUI is a last vestige of graphs in the public interface of Context
// (as opposed to graphs as an implementation detail) intended only for use
// by the "tofu graph" command when asked to render a plan-time graph.
//
// The result of this is intended only for rendering to the user as a dot
// graph, and so may change in future in order to make the result more useful
// in that context, even if drifts away from the physical graph that OpenTofu
// Core currently uses as an implementation detail of planning.
func (c *Context) PlanGraphForUI(config *configs.Config, prevRunState *states.State, mode plans.Mode) (*Graph, tfdiags.Diagnostics) {
	// For now though, this really is just the internal graph, confusing
	// implementation details and all.

	var diags tfdiags.Diagnostics

	opts := &contract.PlanOpts{Mode: mode}

	graph, _, moreDiags := c.planGraph(context.TODO(), config, prevRunState, opts, make(ProviderFunctionMapping))
	diags = diags.Append(moreDiags)
	return graph, diags
}

// All import target addresses with a key must already exist in config.
// When we are able to generate config for expanded resources, this rule can be
// relaxed.
func (c *Context) postExpansionImportValidation(importResolver *ImportResolver, allInst instances.Set) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	for _, importTarget := range importResolver.GetAllImports() {
		if !allInst.HasResourceInstance(importTarget.Addr) {
			diags = diags.Append(importResourceWithoutConfigDiags(importTarget.Addr.String(), nil))
		}
	}
	return diags
}

func (c *Context) postPlanValidateMoves(config *configs.Config, stmts []refactoring.MoveStatement, allInsts instances.Set) tfdiags.Diagnostics {
	return refactoring.ValidateMoves(stmts, config, allInsts)
}

func importResourceWithoutConfigDiags(addressStr string, config *configs.Import) *hcl.Diagnostic {
	diag := hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Configuration for import target does not exist",
		Detail:   fmt.Sprintf("The configuration for the given import %s does not exist. All target instances must have an associated configuration to be imported.", addressStr),
	}

	if config != nil {
		diag.Subject = config.DeclRange.Ptr()
	}

	return &diag
}
