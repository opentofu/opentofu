// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"

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

	glue := &planGlue{
		planCtx:  planCtx,
		targets:  addrs.MakeSet(opts.Targets...),
		excludes: addrs.MakeSet(opts.Excludes...),
	}

	oracle, ctx, diags := configInst.BuildPlanningOracle(ctx, glue)
	if diags.HasErrors() {
		return &plans.Plan{Errored: true}, nil
	}
	// Chicken and egg
	glue.oracle = oracle

	moreDiags := glue.CheckTargets(ctx)
	diags = diags.Append(moreDiags)

	// This oracle.CheckAll call blocks until the evaluator has
	// visited all expressions in the configuration and calls
	// [planContext.PlanDesiredResourceInstance] on the [planGlue] object for
	// each resource instance it discovers so that we can produce a planned
	// action and result value for each one.
	//
	// If this completes without returning any error diagnostics then
	// planCtx.resourceInstObjs should accurately represent the relationships
	// between all of the "current" resource instance objects we found, but
	// we won't discover any deposed objects until the next step below.
	moreDiags = oracle.CheckAll(ctx)
	diags = diags.Append(moreDiags)

	// Record output values and resource dependencies for the plan
	planCtx.rootOutput.Previous = prevRoundState.EnsureModule(addrs.RootModuleInstance).OutputValues
	planCtx.rootOutput.Current = oracle.PlanningResult(ctx).RootModuleOutputs

	intermediate, moreDiags := glue.Finalize(ctx)
	diags = diags.Append(moreDiags)

	plan, moreDiags := finalizePlan(ctx, intermediate, providers)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		plan.Errored = true
	}
	return plan, diags
}
