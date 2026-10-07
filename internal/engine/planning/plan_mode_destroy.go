// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
	"log"
	"sync"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// destroyPlan is the planning implementation for [plans.DestroyMode], dealing
// with the special situation of planning to destroy everything that exists
// in the prior state despite the fact that there are probably some ephemeral
// objects (provider instances, for example) referring to existing objects.
//
// It's tempting to think of "destroy mode" as being equivalent to just removing
// all of the resource instances from the configuration, but that's not quite
// true because destroy mode needs to deal with the unique situation of allowing
// ephemeral objects like provider instances to refer to the prior state of
// objects that are being destroyed, whereas in normal mode that's impossible
// since the resource instances would already have been removed from the
// configuration.
//
// This mode therefore takes quite a different strategy than normal mode where
// we rely on the prior state (after refreshing) as the "result value" for
// each managed resource instance.
func destroyPlan(ctx context.Context, opts *PlanOpts, prevRoundState *states.State, configInst *eval.ConfigInstance, providers plugins.Providers) (*plans.Plan, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	planCtx := newPlanContext(configInst.EvalContext(), prevRoundState, providers, opts)
	planCtx.skipImport = true
	origRefreshOnly := planCtx.refreshOnly
	planCtx.refreshOnly = true

	glue := &planGlueDestroy{
		additionalTargets:  addrs.MakeSet[addrs.Targetable](),
		additionalExcludes: addrs.MakeSet[addrs.Targetable](),
		refreshOnly:        origRefreshOnly,
		normalGlue: planGlue{
			planCtx:  planCtx,
			targets:  addrs.MakeSet(opts.Targets...),
			excludes: addrs.MakeSet(opts.Excludes...),
		},
	}

	oracle, ctx, diags := configInst.BuildPlanningOracle(ctx, glue)
	if diags.HasErrors() {
		return &plans.Plan{Errored: true}, nil
	}
	// Chicken and egg
	glue.normalGlue.oracle = oracle

	moreDiags := glue.normalGlue.CheckTargets(ctx)
	diags = diags.Append(moreDiags)

	// This CheckAll call blocks until the evaluator has
	// visited all expressions in the configuration and calls
	// [planContext.PlanDesiredResourceInstance] on the [planGlue] object for
	// each resource instance it discovers so that we can produce a planned
	// action and result value for each one.
	moreDiags = oracle.CheckAll(ctx)
	diags = diags.Append(moreDiags)

	intermediate, moreDiags := glue.Finalize(ctx)
	diags = diags.Append(moreDiags)

	plan, moreDiags := finalizePlan(ctx, intermediate, providers)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		plan.Errored = true
	}
	return plan, diags
}

// planGlueDestroy is a special variant of [eval.PlanGlue] that models the
// very unusual behavior of the "destroy" planning mode, where we pretend that
// nothing is desired and just plan to destroy everything.
//
// This is a wrapper around [planGlue] so it can share some of its logic for
// planning "orphan" resource instances, but returns quite different answers
// to requests from the evaluator.
type planGlueDestroy struct {
	normalGlue planGlue

	targetingMu        sync.Mutex
	additionalTargets  addrs.Set[addrs.Targetable]
	additionalExcludes addrs.Set[addrs.Targetable]

	refreshOnly bool
}

var _ eval.PlanGlue = (*planGlueDestroy)(nil)

// PlanDesiredResourceInstance implements [eval.PlanGlue].
func (p *planGlueDestroy) PlanDesiredResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance) (cty.Value, tfdiags.Diagnostics) {
	log.Printf("[TRACE] planGlueDestroy.PlanDesiredResourceInstance for %s", inst.Addr)

	// In destroy mode we ignore the evaluator's opinion about what is
	// "desired", handing everything in the various orphan-planning functions
	// instead because in destroy mode absolutely nothing is "desired".
	//
	// But we do still need to produce an upgraded-and-refreshed version of
	// the requested resource instance from the prior state so that the
	// evaluator can use that result for any references to this resource
	// instance that might appear in the configuration for ephemeral objects
	// such as provider instances.

	// FIXME: The behavior that follows is focused on managed resource
	// instances. We need to figure out what we ought to do with data resource
	// instances: just rely on their previous run state here too, or to try
	// to read them?
	// Relying on previous run state is risky because there's no mechanism for
	// "upgrading" the data for a data resource type -- the protocol assumes
	// that we'll be re-reading them from scratch every time anyway.
	// But trying to read them here is also risky because that would force the
	// provider instances they use to be configured as a side-effect, which
	// may then rely on state prior state results for managed resources and
	// so cause a schism where we're relying on a mix of stale and updated
	// values. Also, we'd need to decide what happens if the data resource
	// instance isn't readable during the planning phase.
	// There isn't really any great answer here, but we should study the old
	// runtime's handling of this situation and mimic it as closely as we can
	// for backward-compatibility.

	if inst.Addr.Resource.Resource.Mode == addrs.ManagedResourceMode {
		obj, diags := p.normalGlue.planDesiredManagedResourceInstance(ctx, inst)
		result := obj.ResultValue()
		if p.normalGlue.isTargeting() && !isDeferredVal(result) {
			// If we are targeted, everything we depend on is targeted
			p.targetingMu.Lock()
			// We rely on the config graph to reach all required instances
			// during PreProcess.
			p.additionalTargets.Add(inst.Addr)

			// Config dependencies are already processed here
			for dep := range obj.StateDependencies.All() {
				p.additionalTargets.Add(dep.InstanceAddr)
			}
			p.targetingMu.Unlock()
		}

		return result, diags
	}
	return cty.DynamicVal, nil
}

func (p *planGlueDestroy) Finalize(ctx context.Context) (*planContextResult, tfdiags.Diagnostics) {
	p.targetingMu.Lock()
	defer p.targetingMu.Unlock()

	// This includes deposed, which I believe is correct
	orphaned := resourceInstancesObjects(p.normalGlue.planCtx.prevRoundState)

	if p.normalGlue.isTargeting() {
		// Include additionally discovered config targets for orphaning
		p.normalGlue.targets = p.normalGlue.targets.Union(p.additionalTargets)
		clear(p.additionalTargets)

		// Include additionally discovered state targets for orphaning
		for resource, prevState := range orphaned {
			if p.normalGlue.isTargeted(resource.InstanceAddr) {
				// Already targeted
				continue
			}

			// If our dependencies are targeted, we are targeted as well
			// Assumes flattened dependencies
			for dep := range prevState.TargetDependencies() {
				if p.normalGlue.isTargeted(dep) {
					p.additionalTargets.Add(resource.InstanceAddr)
					break
				}
			}
		}
		p.normalGlue.targets = p.normalGlue.targets.Union(p.additionalTargets)
	}
	if p.normalGlue.isExcluding() {
		// Include additionally discovered config excludes for orphaning
		p.normalGlue.excludes = p.normalGlue.excludes.Union(p.additionalExcludes)
		clear(p.additionalExcludes)

		// Include additionally discovered state excludes for orphaning
		for resource, prevState := range orphaned {
			if !p.normalGlue.isExcluded(resource.InstanceAddr) {
				continue
			}

			// If we are excluded, our dependencies are excluded as well
			// Assumes flattened dependencies
			for dep := range prevState.TargetDependencies() {
				p.additionalExcludes.Add(dep)
			}
		}
		p.normalGlue.excludes = p.normalGlue.excludes.Union(p.additionalExcludes)
	}

	p.normalGlue.planCtx.refreshOnly = p.refreshOnly

	// Recorded moves are typically used to tell a potentially orphaned piece
	// of state that there's a desired instance actually using it through a move.
	// In this scenario, we actually don't want to treat these orphaned pices of state
	// as "moved and used elsewhere".
	recordedMoves := p.normalGlue.planCtx.recordedMoves
	p.normalGlue.planCtx.recordedMoves = addrs.MakeMap[addrs.AbsResourceInstance, addrs.AbsResourceInstance]()

	intermediate, diags := p.normalGlue.Finalize(ctx)
	intermediate.Destroying = true

	// Reset recordedMoves for validation purposes and include additionally discovered moves
	p.normalGlue.planCtx.recordedMoves = recordedMoves.Union(p.normalGlue.planCtx.recordedMoves)

	return intermediate, diags
}
