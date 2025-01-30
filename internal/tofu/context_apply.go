// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"log"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Apply performs the actions described by the given Plan object and returns
// the resulting updated state.
//
// The given configuration *must* be the same configuration that was passed
// earlier to Context.Plan in order to create this plan.
//
// Even if the returned diagnostics contains errors, Apply always returns the
// resulting state which is likely to have been partially-updated.
func (c *Context) Apply(ctx context.Context, plan *plans.Plan, config *configs.Config) (*states.State, tfdiags.Diagnostics) {
	defer c.acquireRun("apply")()

	log.Printf("[DEBUG] Building and walking apply graph for %s plan", plan.UIMode)

	if plan.Errored {
		var diags tfdiags.Diagnostics
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Cannot apply failed plan",
			`The given plan is incomplete due to errors during planning, and so it cannot be applied.`,
		))
		return nil, diags
	}

	for _, rc := range plan.Changes.Resources {
		// Import is a no-op change during an apply (all the real action happens during the plan) but we'd
		// like to show some helpful output that mirrors the way we show other changes.
		if rc.Importing != nil {
			for _, h := range c.hooks {
				// In future, we may need to call PostApplyImport separately elsewhere in the apply
				// operation. For now, though, we'll call Pre and Post hooks together.
				h.PreApplyImport(rc.Addr, *rc.Importing)
				h.PostApplyImport(rc.Addr, *rc.Importing)
			}
		}
	}

	providerFunctionTracker := make(ProviderFunctionMapping)

	graph, operation, diags := c.applyGraph(plan, config, providerFunctionTracker)
	if diags.HasErrors() {
		return nil, diags
	}

	workingState := plan.PriorState.DeepCopy()
	walker, walkDiags := c.walk(ctx, graph, operation, &graphWalkOpts{
		Config:     config,
		InputState: workingState,
		Changes:    plan.Changes,

		// We need to propagate the check results from the plan phase,
		// because that will tell us which checkable objects we're expecting
		// to see updated results from during the apply step.
		PlanTimeCheckResults: plan.Checks,

		// We also want to propagate the timestamp from the plan file.
		PlanTimeTimestamp:       plan.Timestamp,
		ProviderFunctionTracker: providerFunctionTracker,
	})
	diags = diags.Append(walker.NonFatalDiagnostics)
	diags = diags.Append(walkDiags)

	// After the walk is finished, we capture a simplified snapshot of the
	// check result data as part of the new state.
	walker.State.RecordCheckResults(walker.Checks)

	newState := walker.State.Close()
	if plan.UIMode == plans.DestroyMode && !diags.HasErrors() {
		// NOTE: This is a vestigial violation of the rule that we mustn't
		// use plan.UIMode to affect apply-time behavior.
		// We ideally ought to just call newState.PruneResourceHusks
		// unconditionally here, but we historically didn't and haven't yet
		// verified that it'd be safe to do so.
		newState.PruneResourceHusks()
	}

	if len(plan.TargetAddrs) > 0 || len(plan.ExcludeAddrs) > 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Warning,
			"Applied changes may be incomplete",
			`The plan was created with the -target or the -exclude option in effect, so some changes requested in the configuration may have been ignored and the output values may not be fully updated. Run the following command to verify that no other changes are pending:
    tofu plan
	
Note that the -target and -exclude options are not suitable for routine use, and are provided only for exceptional situations such as recovering from errors or mistakes, or when OpenTofu specifically suggests to use it as part of an error message.`,
		))
	}

	// FIXME: we cannot check for an empty plan for refresh-only, because root
	// outputs are always stored as changes. The final condition of the state
	// also depends on some cleanup which happens during the apply walk. It
	// would probably make more sense if applying a refresh-only plan were
	// simply just returning the planned state and checks, but some extra
	// cleanup is going to be needed to make the plan state match what apply
	// would do. For now we can copy the checks over which were overwritten
	// during the apply walk.
	// Despite the intent of UIMode, it must still be used for apply-time
	// differences in destroy plans too, so we can make use of that here as
	// well.
	if plan.UIMode == plans.RefreshOnlyMode {
		newState.CheckResults = plan.Checks.DeepCopy()
	}

	return newState, diags
}

func (c *Context) applyGraph(plan *plans.Plan, config *configs.Config, providerFunctionTracker ProviderFunctionMapping) (*Graph, walkOperation, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	variables := InputValues{}
	for name, dyVal := range plan.VariableValues {
		val, err := dyVal.Decode(cty.DynamicPseudoType)
		if err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid variable value in plan",
				fmt.Sprintf("Invalid value for variable %q recorded in plan file: %s.", name, err),
			))
			continue
		}

		variables[name] = &InputValue{
			Value:      val,
			SourceType: ValueFromPlan,
		}
	}
	if diags.HasErrors() {
		return nil, walkApply, diags
	}

	// The plan.VariableValues field only records variables that were actually
	// set by the caller in the PlanOpts, so we may need to provide
	// placeholders for any other variables that the user didn't set, in
	// which case OpenTofu will once again use the default value from the
	// configuration when we visit these variables during the graph walk.
	for name := range config.Module.Variables {
		if _, ok := variables[name]; ok {
			continue
		}
		variables[name] = &InputValue{
			Value:      cty.NilVal,
			SourceType: ValueFromPlan,
		}
	}

	operation := walkApply
	if plan.UIMode == plans.DestroyMode {
		// FIXME: Due to differences in how objects must be handled in the
		// graph and evaluated during a complete destroy, we must continue to
		// use plans.DestroyMode to switch on this behavior. If all objects
		// which require special destroy handling can be tracked in the plan,
		// then this switch will no longer be needed and we can remove the
		// walkDestroy operation mode.
		// TODO: Audit that and remove walkDestroy as an operation mode.
		operation = walkDestroy
	}

	graph, moreDiags := (&ApplyGraphBuilder{
		Config:                  config,
		Changes:                 plan.Changes,
		State:                   plan.PriorState,
		RootVariableValues:      variables,
		Plugins:                 c.plugins,
		Targets:                 plan.TargetAddrs,
		Excludes:                plan.ExcludeAddrs,
		ForceReplace:            plan.ForceReplaceAddrs,
		Operation:               operation,
		ExternalReferences:      plan.ExternalReferences,
		ProviderFunctionTracker: providerFunctionTracker,
	}).Build(addrs.RootModuleInstance)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, walkApply, diags
	}

	return graph, operation, diags
}

// ApplyGraphForUI is a last vestige of graphs in the public interface of
// Context (as opposed to graphs as an implementation detail) intended only for
// use by the "tofu graph" command when asked to render an apply-time
// graph.
//
// The result of this is intended only for rendering ot the user as a dot
// graph, and so may change in future in order to make the result more useful
// in that context, even if drifts away from the physical graph that OpenTofu
// Core currently uses as an implementation detail of planning.
func (c *Context) ApplyGraphForUI(plan *plans.Plan, config *configs.Config) (*Graph, tfdiags.Diagnostics) {
	// For now though, this really is just the internal graph, confusing
	// implementation details and all.

	var diags tfdiags.Diagnostics

	graph, _, moreDiags := c.applyGraph(plan, config, make(ProviderFunctionMapping))
	diags = diags.Append(moreDiags)
	return graph, diags
}
