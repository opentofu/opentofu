// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
	"fmt"
	"log"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/plans/objchange"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func (p *planGlue) planDesiredManagedResourceInstance(
	ctx context.Context,
	inst *eval.DesiredResourceInstance,
	egb *execGraphBuilder,
) (plannedVal cty.Value, applyResultRef execgraph.ResourceInstanceResultRef, diags tfdiags.Diagnostics) {
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

	validateDiags := p.planCtx.providers.ValidateResourceConfig(ctx, inst.Provider, inst.ResourceMode, inst.ResourceType, inst.ConfigVal)
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
	refreshedPrivate := prevRoundPrivate

	// As long as we have a provider instance we should be able to ask the
	// provider to plan _something_. If this is a placeholder for zero or more
	// instances of a resource whose expansion isn't yet known then we're asking
	// the provider to produce a speculative plan for all of them at once,
	// so we can catch whatever subset of problems are already obvious across
	// all of the potential resource instances.
	planResp := providerClient.PlanResourceChange(ctx, providers.PlanResourceChangeRequest{
		TypeName:         inst.ResourceType,
		PriorState:       refreshedVal,
		ProposedNewState: proposedNewVal,
		Config:           effectiveConfigVal,
		PriorPrivate:     refreshedPrivate,

		// TODO: ProviderMeta is a rarely-used feature that only really makes
		// sense when the module and provider are both written by the same
		// party and the module author is using the provider as a way to
		// transport module usage telemetry. We should decide whether we want
		// to keep supporting that, and if so design a way for the relevant
		// meta value to get from the evaluator into here.
		ProviderMeta: cty.NullVal(cty.DynamicPseudoType),
	})
	for _, err := range objchange.AssertPlanValid(schema.Block, refreshedVal, effectiveConfigVal, planResp.PlannedState) {
		if planResp.LegacyTypeSystem {
			// This provider seems to be using the legacy Terraform plugin SDK
			// that cannot implement the modern protocol correctly, so we'll
			// treat these errors as internal log warnings instead of reporting
			// them. This compromise means that things can work for providers
			// that are only incorrect _because_ they are using the legacy SDK,
			// while still providing some information about the problem in case
			// it's useful for debugging a real issue with a provider.
			//
			// TODO: Bring over the full version of this log message from
			// the original runtime.
			log.Printf("[WARN] Provider produced invalid plan: %s", tfdiags.FormatError(err))
			continue
		}
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

	if eq, _ := planResp.PlannedState.Equals(refreshedVal).Unmark(); !eq.IsKnown() || eq.True() {
		// There is no change to make, so we can return early without adding
		// anything to the graph at all. There will be items in the execgraph
		// for this node only if some other resource instance or provider
		// instance depends on our result, in which case an op to read the
		// prior state should get implicitly added to the graph during the
		// handling of that downstream thing.
		return refreshedVal, execgraph.NilResultRef[*exec.ResourceInstanceObject](), diags
	}

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
		Addr:        inst.Addr,
		PrevRunAddr: inst.Addr, // TODO: If we add "moved" support above then this must record the original address
		ProviderAddr: addrs.AbsProviderConfig{
			// FIXME: This is a lossy shim to the old-style provider instance
			// address representation, since our old models aren't yet updated
			// to support the modern one. It cannot handle a provider config
			// inside a module call that uses count or for_each.
			Module:   (*inst.ProviderInstance).Config.Module.Module(),
			Provider: (*inst.ProviderInstance).Config.Config.Provider,
			Alias:    (*inst.ProviderInstance).Config.Config.Alias,
		},
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

	finalResultRef := p.managedResourceInstanceExecSubgraph(
		ctx,
		plannedChange,
		inst.ProviderInstance,
		inst.RequiredResourceInstances,
		egb,
	)

	// Our result value for ongoing downstream planning is the planned new state.
	return planResp.PlannedState, finalResultRef, diags
}

func (p *planGlue) planOrphanManagedResourceInstance(
	ctx context.Context,
	addr addrs.AbsResourceInstance,
	stateSrc *states.ResourceInstanceObjectFullSrc,
	egb *execGraphBuilder,
) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	// TODO: This currently has a lot of inline logic that's quite similar to
	// what's in [planGlue.planDesiredManagedResourceInstance]. Once we're
	// satisfied that this set of methods is feature-complete we should consider
	// how to factor out as much of this logic as possible into shared functions
	// so that this'll be easier to maintain in future as requirements change.

	// TODO: Ask the planning oracle whether there are any "moved" blocks
	// that begin at inst.Addr, and if so check whether the chain of moves
	// starting there will end up at a currently-unbound resource instance
	// address. If so, we should do nothing here because
	// [planGlue.planOrphanManagedResourceInstance] for that target address
	// should notice the opposite end of the same chain of moves and so
	// handle it as an object that is in both the prior and desired state,
	// albeit with different addresses in each.

	providerAddr := stateSrc.ProviderInstanceAddr.Config.Config.Provider
	evalCtx := p.oracle.EvalContext(ctx)
	schema, schemaDiags := evalCtx.Providers.ResourceTypeSchema(ctx,
		providerAddr,
		addr.Resource.Resource.Mode,
		addr.Resource.Resource.Type,
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
				addr, providerAddr, addr.Resource.Resource.Type,
			),
			nil, // this error belongs to the whole resource config
		))
		return diags
	}

	var prevRoundVal cty.Value
	var prevRoundPrivate []byte
	prevRoundState, err := states.DecodeResourceInstanceObjectFull(stateSrc, schema.Block.ImpliedType())
	if err != nil {
		diags = diags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Invalid prior state for resource instance",
			fmt.Sprintf(
				"Cannot decode the most recent state snapshot for %s: %s.\n\nIs the selected version of %s incompatible with the provider that most recently changed this object?",
				addr, tfdiags.FormatError(err), providerAddr,
			),
			nil, // this error belongs to the whole resource config
		))
		return diags
	}
	prevRoundVal = prevRoundState.Value
	prevRoundPrivate = prevRoundState.Private

	// FIXME: Currently this fails if the only mention of a particular provider
	// instance is in the state, because this function relies on provider
	// config information from the evaluator and thus only from the config.
	// If you get the error about the provider not being able to initialize
	// then you might currently need to add an explicit empty provider config
	// block for the provider, if you were testing with a provider like
	// hashicorp/null where an explicit configuration is not normally required.
	//
	// There's another FIXME comment further down the callstack beneath this
	// function identifying the main location of the problem.
	providerClient, moreDiags := p.providerClient(ctx, prevRoundState.ProviderInstanceAddr)
	if providerClient == nil {
		moreDiags = moreDiags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Provider instance not available",
			fmt.Sprintf("Cannot plan %s because its associated provider instance %s cannot initialize.", addr, prevRoundState.ProviderInstanceAddr),
			nil,
		))
	}
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return diags
	}

	// TODO: Call providerClient.ReadResource and update the "refreshed state"
	// and reassign this refreshedVal to the refreshed result.
	refreshedVal := prevRoundVal
	refreshedPrivate := prevRoundPrivate

	if refreshedVal.IsNull() {
		// The orphan object seems to have already been deleted outside of
		// OpenTofu, so we've got nothing more to do here.
		return diags
	}

	newVal := cty.NullVal(schema.Block.ImpliedType())
	// FIXME: Whether the provider gets involved in planning to delete something
	// is a dynamically-negotiable protocol feature, with older providers unable
	// to handle requests like this. We ought to skip the following call unless
	// we know the provider has negotiated the relevant capability.
	planResp := providerClient.PlanResourceChange(ctx, providers.PlanResourceChangeRequest{
		TypeName:         addr.Resource.Resource.Type,
		PriorState:       refreshedVal,
		ProposedNewState: newVal,
		Config:           newVal,
		PriorPrivate:     refreshedPrivate,

		// TODO: ProviderMeta is a rarely-used feature that only really makes
		// sense when the module and provider are both written by the same
		// party and the module author is using the provider as a way to
		// transport module usage telemetry. We should decide whether we want
		// to keep supporting that, and if so design a way for the relevant
		// meta value to get from the evaluator into here.
		ProviderMeta: cty.NullVal(cty.DynamicPseudoType),
	})
	// TODO: Check that the provider's planned value is compatible with what
	// we sent in Config, which was a null value and therefore the planned
	// value must also be null.

	plannedChange := &plans.ResourceInstanceChange{
		Addr:        addr,
		PrevRunAddr: addr,
		ProviderAddr: addrs.AbsProviderConfig{
			// FIXME: This is a lossy shim to the old-style provider instance
			// address representation, since our old models aren't yet updated
			// to support the modern one. It cannot handle a provider config
			// inside a module call that uses count or for_each.
			Module:   prevRoundState.ProviderInstanceAddr.Config.Module.Module(),
			Provider: prevRoundState.ProviderInstanceAddr.Config.Config.Provider,
			Alias:    prevRoundState.ProviderInstanceAddr.Config.Config.Alias,
		},
		RequiredReplace: cty.NewPathSet(planResp.RequiresReplace...),
		Private:         planResp.PlannedPrivate,
		Change: plans.Change{
			Action: plans.Delete,
			Before: refreshedVal,
			After:  planResp.PlannedState,
		},

		// TODO: ActionReason, but need to figure out how to get the information
		// we'd need for that into here. For example, to report that the
		// instance address is no longer in the configuration we need to be
		// able to refer to the configuration in here. Or maybe our caller
		// should just pass in a reason as an additonal argument to this
		// function, since it presumably already knows how it concluded that
		// this address is "orphaned".
	}
	plannedChangeSrc, err := plannedChange.Encode(schema.Block.ImpliedType())
	if err != nil {
		// TODO: Make a proper error diagnostic for this, like the original
		// runtime does.
		diags = diags.Append(err)
		return diags
	}
	p.planCtx.plannedChanges.AppendResourceInstanceChange(plannedChangeSrc)

	// FIXME: Our state model currently tracks dependencies between whole
	// resources rather than between individual instances of resources, so
	// we can't actually populate the requiredResourceInstances argument
	// correctly right now. To do that we'll need to update the state model
	// to be able to track individual resource instances and introduce some
	// shim behavior in the state decoder so that it'll automatically translate
	// whole-resource dependencies into per-instance dependencies based on
	// which instances are present for each resource in the whole previous run
	// state snapshot.
	var requiredResourceInstances addrs.Set[addrs.AbsResourceInstance]
	p.managedResourceInstanceExecSubgraph(
		ctx,
		plannedChange,
		&prevRoundState.ProviderInstanceAddr,
		requiredResourceInstances,
		egb,
	)

	return diags
}

func (p *planGlue) planDeposedManagedResourceInstanceObject(
	ctx context.Context,
	addr addrs.AbsResourceInstance,
	deposedKey states.DeposedKey,
	state *states.ResourceInstanceObjectFullSrc,
	egb *execGraphBuilder,
) tfdiags.Diagnostics {
	// TODO: Implement
	panic("unimplemented")
}

// managedResourceInstanceExecSubgraph prepares what's needed to include
// changes for a managed resource instance in an execution graph and then
// adds the relevant nodes, returning a result reference referring to the
// final result of the apply steps.
//
// This is a small wrapper around [execGraphBuilder.ManagedResourceInstanceSubgraph]
// which implicitly adds execgraph items needed for the resource instance's
// provider instance, which requires some information that an [execGraphBuilder]
// instance cannot access directly itself.
//
// Note that a nil pointer for providerInstAddr is currently how we represent
// that we don't know which provider instance address to use. We'll hopefully
// do something clearer than that in future; refer to related FIXME comments in
// [planGlue.ensureProviderInstanceExecgraph] for more information.
// TODO: remove this paragraph once we've switched to a better representation.
//
// FIXME: Because we're currently still using our old model for describing
// planned changes, we need to bring a little extra baggage alongside the
// change object in our arguments here. As we start to design a new model
// for describing planned changes in future we should aspire for this function
// to take only (ctx, plannedChange, egb) as arguments and have the
// plannedChange object be a self-contained representation of everything needed
// to make the graph-building decisions.
func (p *planGlue) managedResourceInstanceExecSubgraph(
	ctx context.Context,
	plannedChange *plans.ResourceInstanceChange,
	providerInstAddr *addrs.AbsProviderInstanceCorrect,
	requiredResourceInstances addrs.Set[addrs.AbsResourceInstance],
	egb *execGraphBuilder,
) execgraph.ResourceInstanceResultRef {
	providerClientRef, registerProviderCloseBlocker := p.ensureProviderInstanceExecgraph(ctx, providerInstAddr, egb)
	finalResultRef := egb.ManagedResourceInstanceSubgraph(plannedChange, providerClientRef, requiredResourceInstances)
	registerProviderCloseBlocker(finalResultRef)
	return finalResultRef
}
