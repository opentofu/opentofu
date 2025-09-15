// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"

	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// PlanChanges is the main entry point, taking a state snapshot from the end
// of the previous plan/apply round and an instantiated configuration (bound
// to some input variable definitions) and returning a plan containing a set of
// proposed actions.
//
// This is currently really just a placeholder to demonstrate the role that the
// functionality in lang/eval might play in a planning process and what other
// work would need to happen in a planning engine that is beyond the scope
// of lang/eval. For now then it's just using our existing models of state
// and plan, but our larger ambitions also involve some other changes that
// would likely cause the signature here to change significantly:
//
//   - We're considering changing the apply phase implementation to just be a
//     walk of an execution graph calculated during the planning phase, which
//     implies that the "plan" model would need to change significantly to
//     be able to directly represent that graph, whereas the current model only
//     _implies_ that graph at a high level while expecting the apply phase
//     itself to construct the finalized graph.
//   - We're considering switching from a "state snapshot" model to a more
//     granular model where we request individual objects from the state storage
//     as needed, in which case we'd likely change our usage pattern so that
//     the planning phase is able to create a "provider-like" live object that
//     offers an API for fetching items from the state as needed, rather than
//     the current pure-data snapshot representation.
//
// Therefore readers of this code should focus mainly on the inner
// implementation of how it decides what to plan and how to plan it, and less
// on where it gets the information to make those decisions and how it
// represents those decisions in its return value.
func PlanChanges(ctx context.Context, prevRoundState *states.State, configInst *eval.ConfigInstance) (*plans.Plan, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	planCtx := newPlanContext(configInst.EvalContext(), prevRoundState)

	// This configInst.DrivePlanning call blocks until the evaluator has
	// visited all expressions in the configuration and calls
	// [planContext.PlanDesiredResourceInstance] on planCtx for each resource
	// instance it discovers so that we can produce a planned action and
	// result value for each one.
	//
	// It also calls the various "Plan*Orphans" methods at different levels
	// of granularity once it's determined the full set of objects under
	// a given prefix, which planCtx uses to notice when there are
	// prevRoundState resource instances that are no longer in the desired
	// state and so plan to delete or forget them.
	_, moreDiags := configInst.DrivePlanning(ctx, planCtx)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		// If we encountered errors during the eval-based phase then we'll halt
		// here but we'll still produce a best-effort [plans.Plan] describing
		// the situation because that often gives useful information for debugging
		// what caused the errors.
		plan := planCtx.Close()
		plan.Errored = true
		return plan, diags
	}

	// TODO: After configInst.DrivePlanning has finished we should plan "delete"
	// actions for any deposed objects in the previous round state, which need
	// to get deleted regardless of what the configuration says. This means
	// that we'll need to make planCtx aware of which provider instances are
	// used by those deposed objects so that it can leave those instances
	// open for us to use in this followup step.
	//
	// It's a little annoying to need to leave provider instances (and any
	// ephemeral resource instances they depend on) open for longer than normal
	// in this case, but deposed objects in the previous run state are
	// relatively rare -- it can only occur if a previous round failed to
	// destroy them during a create_before_destroy "replace" -- and so this
	// seems like a reasonable concession to avoid complicating the eval system
	// itself with knowledge about deposed objects.

	return planCtx.Close(), diags
}
