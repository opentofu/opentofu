package graph

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu/variables"
)

func (c *Context) Apply(ctx context.Context, plan *plans.Plan, config *configs.Config, setVariables variables.InputValues) (*states.State, tfdiags.Diagnostics) {
	defer c.acquireRun("apply")()

	providerFunctionTracker := make(ProviderFunctionMapping)

	graph, operation, diags := c.applyGraph(ctx, plan, config, providerFunctionTracker, setVariables)
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

	return walker.State.Close(), diags

}

func (c *Context) applyGraph(ctx context.Context, plan *plans.Plan, config *configs.Config, providerFunctionTracker ProviderFunctionMapping, setVariables variables.InputValues) (*Graph, walkOperation, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

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
		RootVariableValues:      setVariables,
		Plugins:                 c.plugins,
		Targets:                 plan.TargetAddrs,
		Excludes:                plan.ExcludeAddrs,
		ForceReplace:            plan.ForceReplaceAddrs,
		Operation:               operation,
		ExternalReferences:      plan.ExternalReferences,
		ProviderFunctionTracker: providerFunctionTracker,
	}).Build(ctx, addrs.RootModuleInstance)
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
// The result of this is intended only for rendering to the user as a dot
// graph, and so may change in future in order to make the result more useful
// in that context, even if drifts away from the physical graph that OpenTofu
// Core currently uses as an implementation detail of planning.
func (c *Context) ApplyGraphForUI(plan *plans.Plan, config *configs.Config) (*Graph, tfdiags.Diagnostics) {
	// For now though, this really is just the internal graph, confusing
	// implementation details and all.

	var diags tfdiags.Diagnostics

	graph, _, moreDiags := c.applyGraph(context.TODO(), plan, config, make(ProviderFunctionMapping), nil)
	diags = diags.Append(moreDiags)
	return graph, diags
}
