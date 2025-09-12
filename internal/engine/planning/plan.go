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

	// Our planning process has two main "sub-phases":
	// - Planning create, update, or replace actions for resource instances that
	//   are currently declared in the configuration.
	// - Planning delete actions for resource instances that appear only in
	//   the previous round state and are not declared in the configuration.
	//
	// The lang/eval package is the primary driver of the first sub-phase,
	// because it knows how to gradually evaluate expressions in the
	// configuration until it has discovered the full set of "desired" resource
	// instances.
	//
	// The second phase is outside of lang/eval's scope, handled entirely
	// within the planning engine: we subtract the set of resource instances
	// we found during the first sub-phase from the set of resource instances
	// in the previous round state and plan to destroy whichever resource
	// instances remain. The second sub-phase also uses provider instance
	// configuration and ephemeral resource instance configuration provided
	// by lang/eval through the return value of the first phase.
	//
	// Note that this approach implies that any provider instance that is
	// associated with a resource instance in prevRoundState and which
	// gets used during subphase 1 must always be kept open through the
	// remainder of subphase 1 _just in case_ any of the resource instances
	// that refer to it turn out to be "orphans". In turn that means that
	// any ephemeral resource instances that any of those provider instances
	// rely on must also remain open, and in turn the provider instances that
	// those ephemeral resource instances belong to. So we would probably end
	// up keeping most of the provider instances open throughout anyway and
	// so maybe it's not worth the complexity of trying to calculate when it's
	// okay to close them. If we _do_ want to still track this precisely then
	// we'll need to extend the PlanGlue API to include additional announcements
	// about what's present in the configuration so that the orphan planning
	// can happen concurrently with the desired state planning.

	planCtx := newPlanContext(configInst.EvalContext(), prevRoundState)

	// Sub-phase 1: evaluating the desired state
	//
	// This configInst.DrivePlanning call blocks until the evaluator has
	// visited all expressions in the configuration and calls
	// [planContext.PlanDesiredResourceInstance] on planCtx for each resource
	// instance it discovers so that we can produce a planned action and
	// result value for each one.
	result, moreDiags := configInst.DrivePlanning(ctx, planCtx)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		// If we encountered errors during the eval-based phase then we'll halt
		// here but we'll still produce a best-effort [plans.Plan] describing
		// the situation because that often gives useful information for debugging
		// what caused the errors.
		plan := planCtx.Plan()
		plan.Errored = true
		return plan, diags
	}

	// Sub-phase 2: orphaned resource instances
	//
	// We'll continue to use the [eval.PlanningOracle] in case we need to
	// open any new provider instances or ephemeral resource instances to
	// deal with the deletion of any "orphan" resource instances.
	oracle := result.Oracle
	moreDiags = planCtx.PlanOrphanResourceInstances(ctx, oracle)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		plan := planCtx.Plan()
		plan.Errored = true
		return plan, diags
	}

	return planCtx.Plan(), diags
}
