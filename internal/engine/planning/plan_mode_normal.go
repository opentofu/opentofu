// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// normalPlan is the planning implementation for [plans.NormalMode], dealing
// with the default case of planning to change remote objects to better match
// the desired state described by the current configuration.
func normalPlan(ctx context.Context, opts *PlanOpts, prevRoundState *states.State, configInst *eval.ConfigInstance, providers plugins.Providers) (*plans.Plan, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	planCtx := newPlanContext(configInst.EvalContext(), prevRoundState, providers, opts)

	// This configInst.DrivePlanning call blocks until the evaluator has
	// visited all expressions in the configuration and calls
	// [planContext.PlanDesiredResourceInstance] on the [planGlue] object for
	// each resource instance it discovers so that we can produce a planned
	// action and result value for each one.
	//
	// It also calls the various "Plan*Orphans" methods at different levels
	// of granularity once it's determined the full set of objects under
	// a given prefix, which planGlue uses to notice when there are
	// prevRoundState resource instances that are no longer in the desired
	// state and so plan to delete or forget them.
	//
	// If this completes without returning any error diagnostics then
	// planCtx.resourceInstObjs should accurately represent the relationships
	// between all of the "current" resource instance objects we found, but
	// we won't discover any deposed objects until the next step below.
	evalResult, moreDiags := configInst.DrivePlanning(ctx, &planGlue{
		planCtx:  planCtx,
		targets:  addrs.MakeSet(opts.Targets...),
		excludes: addrs.MakeSet(opts.Excludes...),
	})
	diags = diags.Append(moreDiags)
	if evalResult == nil {
		if !moreDiags.HasErrors() {
			// This should not happen: we should always have an evalResult if
			// there weren't any errors.
			panic(fmt.Sprintf("%T.DrivePlanning returned nil result without any error diagnostics", configInst))
		}
	} else {

		// Record output values and resource dependencies for the plan
		planCtx.rootOutput.Previous = prevRoundState.EnsureModule(addrs.RootModuleInstance).OutputValues
		planCtx.rootOutput.Current = evalResult.RootModuleOutputs
	}

	// TODO: Consider factoring most of the work we've done here into a single
	// function that directly returns the "intermediate" object. Exposing
	// planCtx as a mutable object in this function doesn't seem necessary
	// anymore since we only actually care about the results from Close here.
	intermediate, moreDiags := planCtx.Close(ctx)
	diags = diags.Append(moreDiags)
	plan, moreDiags := finalizePlan(ctx, intermediate, providers)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		plan.Errored = true
	}
	return plan, diags
}
