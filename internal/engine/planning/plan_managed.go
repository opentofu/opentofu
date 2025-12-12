// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
	"fmt"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/plans/objchange"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func (p *planGlue) planDesiredManagedResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance, egb *execgraph.Builder) (plannedVal cty.Value, applyResultRef execgraph.ResourceInstanceResultRef, diags tfdiags.Diagnostics) {
	// Regardless of outcome we'll always report that we completed planning.
	defer p.planCtx.reportResourceInstancePlanCompletion(inst.Addr)

	// There are various reasons why we might need to defer final planning
	// of this to a later round. The following is not exhaustive but is a
	// placeholder to show where deferral might fit in.
	if p.desiredResourceInstanceMustBeDeferred(inst) {
		p.planCtx.deferred.Put(inst.Addr, struct{}{})
		defer func() {
			// Our result must be marked as deferred, whichever return path
			// we leave through.
			if plannedVal != cty.NilVal {
				plannedVal = deferredVal(plannedVal)
			}
		}()
		// We intentionally continue anyway, because we'll make a best effort
		// to produce a speculative plan based on the information we _do_ know
		// in case that allows us to detect a problem sooner. The important
		// thing is that in the deferred case we won't actually propose any
		// planned changes for this resource instance.
	}

	evalCtx := p.oracle.EvalContext(ctx)
	schema, schemaDiags := evalCtx.Providers.ResourceTypeSchema(ctx,
		inst.Provider,
		inst.Addr.Resource.Resource.Mode,
		inst.Addr.Resource.Resource.Type,
	)
	if schemaDiags.HasErrors() {
		// We don't return the schema-loading diagnostics directly here because
		// they should have already been returned by earlier code, but we do
		// return a more specific error to make it clear that this specific
		// resource instance was unplannable because of the problem.
		diags = diags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Resource type schema unavailable",
			fmt.Sprintf(
				"Cannot plan %s because provider %s failed to return the schema for its resource type %q.",
				inst.Addr, inst.Provider, inst.Addr.Resource.Resource.Type,
			),
			nil, // this error belongs to the whole resource config
		))
		return cty.DynamicVal, nil, diags
	}

	validateDiags := p.planCtx.providers.ValidateResourceConfig(ctx, inst.Provider, inst.Addr.Resource.Resource.Mode, inst.Addr.Resource.Resource.Type, inst.ConfigVal)
	diags = diags.Append(validateDiags)
	if diags.HasErrors() {
		return cty.DynamicVal, nil, diags
	}

	var prevRoundVal cty.Value
	var prevRoundPrivate []byte
	prevRoundState := p.planCtx.prevRoundState.SyncWrapper().ResourceInstanceObjectFull(inst.Addr, states.NotDeposed)
	if prevRoundState != nil {
		obj, err := states.DecodeResourceInstanceObjectFull(prevRoundState, schema.Block.ImpliedType())
		if err != nil {
			diags = diags.Append(tfdiags.AttributeValue(
				tfdiags.Error,
				"Invalid prior state for resource instance",
				fmt.Sprintf(
					"Cannot decode the most recent state snapshot for %s: %s.\n\nIs the selected version of %s incompatible with the provider that most recently changed this object?",
					inst.Addr, tfdiags.FormatError(err), inst.Provider,
				),
				nil, // this error belongs to the whole resource config
			))
			return cty.DynamicVal, nil, diags
		}
		prevRoundVal = obj.Value
		prevRoundPrivate = obj.Private
	} else {
		// TODO: Ask the planning oracle whether there are any "moved" blocks
		// that ultimately end up at inst.Addr (possibly through a chain of
		// multiple moves) and check the source instance address of each
		// one in turn in case we find an as-yet-unclaimed resource instance
		// that wants to be rebound to the address in inst.Addr.
		// (Note that by handling moved blocks at _this_ point we could
		// potentially have the eval system allow dynamic instance keys etc,
		// which the original runtime can't do because it always deals with
		// moved blocks as a preprocessing step before doing other work.)
		prevRoundVal = cty.NullVal(schema.Block.ImpliedType())
	}

	proposedNewVal := p.resourceInstancePlaceholderValue(ctx,
		inst.Provider,
		inst.Addr.Resource.Resource.Mode,
		inst.Addr.Resource.Resource.Type,
		prevRoundVal,
		inst.ConfigVal,
	)

	if inst.ProviderInstance == nil {
		// If we don't even know which provider instance we're supposed to be
		// talking to then we'll just return a placeholder value, because
		// we don't have any way to generate a speculative plan.
		return proposedNewVal, nil, diags
	}

	providerClient, moreDiags := p.providerClient(ctx, *inst.ProviderInstance)
	if providerClient == nil {
		moreDiags = moreDiags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Provider instance not available",
			fmt.Sprintf("Cannot plan %s because its associated provider instance %s cannot initialize.", inst.Addr, *inst.ProviderInstance),
			nil,
		))
	}
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return proposedNewVal, nil, diags
	}

	// TODO: If inst.IgnoreChangesPaths has any entries then we need to
	// transform effectiveConfigVal so that any paths specified in there are
	// forced to match the corresponding value from prevRoundVal, if any.
	effectiveConfigVal := inst.ConfigVal

	// TODO: Call providerClient.ReadResource and update the "refreshed state"
	// and reassign this refreshedVal to the refreshed result.
	refreshedVal := prevRoundVal

	// As long as we have a provider instance we should be able to ask the
	// provider to plan _something_. If this is a placeholder for zero or more
	// instances of a resource whose expansion isn't yet known then we're asking
	// the provider to produce a speculative plan for all of them at once,
	// so we can catch whatever subset of problems are already obvious across
	// all of the potential resource instances.
	planResp := providerClient.PlanResourceChange(ctx, providers.PlanResourceChangeRequest{
		TypeName:         inst.Addr.Resource.Resource.Type,
		PriorState:       refreshedVal,
		ProposedNewState: proposedNewVal,
		Config:           effectiveConfigVal,
		PriorPrivate:     prevRoundPrivate,
		// TODO: ProviderMeta
	})
	for _, err := range objchange.AssertPlanValid(schema.Block, refreshedVal, effectiveConfigVal, planResp.PlannedState) {
		// TODO: If resp.LegacyTypeSystem is set then we should generate
		// warnings in the log but continue anyway, like the original
		// runtime does.
		planResp.Diagnostics = planResp.Diagnostics.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Provider produced invalid plan",
			// TODO: Bring over the full version of this error case from the
			// original runtime.
			fmt.Sprintf("Invalid planned new value: %s.", tfdiags.FormatError(err)),
			nil,
		))
	}
	diags = diags.Append(planResp.Diagnostics)
	if planResp.Diagnostics.HasErrors() {
		return proposedNewVal, nil, diags
	}

	// TODO: Check for resp.Deferred once we've updated package providers to
	// include it. If that's set then the _provider_ is telling us we must
	// defer planning any action for this resource instance. We'd still
	// return the planned new state as a placeholder for downstream planning in
	// that case, but we would need to mark it as deferred and _not_ record a
	// proposed change for it.

	plannedAction := plans.Update
	if prevRoundState == nil {
		plannedAction = plans.Create
	} else if len(planResp.RequiresReplace) != 0 {
		if inst.CreateBeforeDestroy {
			plannedAction = plans.CreateThenDelete
		} else {
			plannedAction = plans.DeleteThenCreate
		}
	}
	// (a "desired" object cannot have a Delete action; we handle those cases
	// in planOrphanManagedResourceInstance and planDeposedManagedResourceInstanceObject below.)
	plannedChange := &plans.ResourceInstanceChange{
		Addr:            inst.Addr,
		PrevRunAddr:     inst.Addr,                 // TODO: If we add "moved" support above then this must record the original address
		ProviderAddr:    addrs.AbsProviderConfig{}, // FIXME: Old models are using the not-quite-correct provider address types, so we can't populate this properly
		RequiredReplace: cty.NewPathSet(planResp.RequiresReplace...),
		Private:         planResp.PlannedPrivate,
		Change: plans.Change{
			Action: plannedAction,
			Before: refreshedVal,
			After:  planResp.PlannedState,
		},

		// TODO: ActionReason, but need to figure out how to get the information
		// we'd need for that into here since most of the reasons are
		// configuration-related and so would need to be driven by stuff in
		// [eval.DesiredResourceInstance].
	}
	plannedChangeSrc, err := plannedChange.Encode(schema.Block.ImpliedType())
	if err != nil {
		// TODO: Make a proper error diagnostic for this, like the original
		// runtime does.
		diags = diags.Append(err)
		return planResp.PlannedState, nil, diags
	}
	p.planCtx.plannedChanges.AppendResourceInstanceChange(plannedChangeSrc)

	// The following is a placeholder for execgraph construction, which isn't
	// fully wired in yet but is here just to help us understand whether we
	// have enough graph builder and execgraph functionality for this to switch
	// to using execution graphs more completely in later work.
	//
	// FIXME: If this is one of the "replace" actions then we need to generate
	// a more complex graph that has two pairs of "final plan" and "apply".
	providerClientRef, closeProviderAfter := egb.ProviderInstance(*inst.ProviderInstance, egb.Waiter())
	priorStateRef := egb.ResourceInstancePriorState(inst.Addr)
	plannedValRef := egb.ConstantValue(planResp.PlannedState)
	desiredInstRef := egb.DesiredResourceInstance(inst.Addr)
	finalPlanRef := egb.ManagedResourceObjectFinalPlan(
		desiredInstRef,
		priorStateRef,
		plannedValRef,
		providerClientRef,
		egb.Waiter( /* TODO: The final result refs for all of the other resource instances we depend on. */ ),
	)
	finalResultRef := egb.ApplyManagedResourceObjectChanges(
		finalPlanRef,
		providerClientRef,
	)
	closeProviderAfter(finalResultRef)

	// Our result value for ongoing downstream planning is the planned new state.
	return planResp.PlannedState, finalResultRef, diags
}

func (p *planGlue) planOrphanManagedResourceInstance(ctx context.Context, addr addrs.AbsResourceInstance, state *states.ResourceInstanceObjectFullSrc, egb *execgraph.Builder) tfdiags.Diagnostics {
	// Regardless of outcome we'll always report that we completed planning.
	defer p.planCtx.reportResourceInstancePlanCompletion(addr)

	// TODO: Implement
	panic("unimplemented")
}

func (p *planGlue) planDeposedManagedResourceInstanceObject(ctx context.Context, addr addrs.AbsResourceInstance, deposedKey states.DeposedKey, state *states.ResourceInstanceObjectFullSrc, egb *execgraph.Builder) tfdiags.Diagnostics {
	// Regardless of outcome we'll always report that we completed planning.
	defer p.planCtx.reportResourceInstanceDeposedPlanCompletion(addr, deposedKey)

	// TODO: Implement
	panic("unimplemented")
}
