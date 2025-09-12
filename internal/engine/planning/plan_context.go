package planning

import (
	"context"
	"fmt"
	"iter"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// planContext is our shared state for the various parts of a single call
// to [PlanChanges], and also serves as our [eval.PlanGlue] implementation
// through which the evaluator calls us to ask for planning results.
type planContext struct {
	evalCtx        *eval.EvalContext
	plannedChanges *plans.ChangesSync

	// prevRoundState MUST be treated as immutable
	prevRoundState *states.State

	// refreshedState is where we record the results of refreshing
	// resource instances as we visit them. This starts as a deep copy
	// of prevRoundState.
	refreshedState *states.SyncState

	// TODO: something to track which provider instances and which ephemeral
	// resource instances are currently open.
}

var _ eval.PlanGlue = (*planContext)(nil)

func newPlanContext(evalCtx *eval.EvalContext, prevRoundState *states.State) *planContext {
	if prevRoundState == nil {
		prevRoundState = states.NewState()
	}
	changes := plans.NewChanges()
	refreshedState := prevRoundState.DeepCopy()

	return &planContext{
		evalCtx:        evalCtx,
		plannedChanges: changes.SyncWrapper(),
		prevRoundState: prevRoundState,
		refreshedState: refreshedState.SyncWrapper(),
	}
}

// PlanDesiredResourceInstance implements eval.PlanGlue.
//
// This is called each time the evaluation system discovers a new resource
// instance in the configuration, and there are likely to be multiple calls
// active concurrently and so this function must take care to avoid races.
func (p *planContext) PlanDesiredResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance, oracle *eval.PlanningOracle) (cty.Value, tfdiags.Diagnostics) {
	// The details of how we plan vary considerably depending on the resource
	// mode, so we'll dispatch each one to a separate function before we do
	// anything else.
	switch mode := inst.Addr.Resource.Resource.Mode; mode {
	case addrs.ManagedResourceMode:
		return p.planDesiredManagedResourceInstance(ctx, inst, oracle)
	case addrs.DataResourceMode:
		return p.planDesiredDataResourceInstance(ctx, inst, oracle)
	case addrs.EphemeralResourceMode:
		return p.planDesiredEphemeralResourceInstance(ctx, inst, oracle)
	default:
		// We should not get here because the cases above should always be
		// exhaustive for all of the valid resource modes.
		var diags tfdiags.Diagnostics
		diags = diags.Append(fmt.Errorf("the planning engine does not support %s; this is a bug in OpenTofu", mode))
		return cty.DynamicVal, diags
	}
}

// PlanModuleCallInstanceOrphans implements eval.PlanGlue.
func (p *planContext) PlanModuleCallInstanceOrphans(ctx context.Context, moduleCallAddr addrs.AbsModuleCall, desiredInstances iter.Seq[addrs.InstanceKey]) tfdiags.Diagnostics {
	panic("unimplemented")
}

// PlanModuleCallOrphans implements eval.PlanGlue.
func (p *planContext) PlanModuleCallOrphans(ctx context.Context, callerModuleInstAddr addrs.ModuleInstance, desiredCalls iter.Seq[addrs.ModuleCall]) tfdiags.Diagnostics {
	panic("unimplemented")
}

// PlanResourceInstanceOrphans implements eval.PlanGlue.
func (p *planContext) PlanResourceInstanceOrphans(ctx context.Context, resourceAddr addrs.AbsResource, desiredInstances iter.Seq[addrs.InstanceKey]) tfdiags.Diagnostics {
	panic("unimplemented")
}

// PlanResourceOrphans implements eval.PlanGlue.
func (p *planContext) PlanResourceOrphans(ctx context.Context, moduleInstAddr addrs.ModuleInstance, desiredResources iter.Seq[addrs.Resource]) tfdiags.Diagnostics {
	panic("unimplemented")
}

func (p *planContext) Plan() *plans.Plan {
	return &plans.Plan{
		UIMode:       plans.NormalMode, // TODO: This PlanChanges function needs something analogous to [tofu.PlanOpts] for planning mode/options
		Changes:      p.plannedChanges.Close(),
		PrevRunState: p.prevRoundState,
		PriorState:   p.refreshedState.Close(),
		// TODO: various other fields that we need to actually make use
		// of this plan result. But this is intentionally just a partial
		// result for now because it's not clear that we'd even be using
		// plans.Plan in a final version of this new approach.
	}
}
