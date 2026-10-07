// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
	"fmt"
	"log"
	"slices"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/resources"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func (p *planGlue) processMoveOrUpgrade(
	ctx context.Context,
	addr addrs.AbsResourceInstanceObject,
	writeStateAddr addrs.AbsResourceInstanceObject,
	provider addrs.Provider,
	resourceType *resources.ManagedResourceType,
	stateInfo moveWithState,
) (*resources.EncodedValueWithPrivate, *states.ResourceInstanceObjectFullSrc, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	tracer := contextTracer(ctx)
	prevRoundState := stateInfo.state

	var updatedState *resources.EncodedValueWithPrivate
	isMoveToNewType := stateInfo.isMove() && !stateInfo.isImplicit() && (resourceType.ResourceTypeName() != prevRoundState.ResourceType || !provider.Equals(prevRoundState.ProviderInstanceAddr.Config.Config.Provider))
	if isMoveToNewType {
		// Moved to a new resource type
		moveCtx := ctx
		if cb := tracer.StartManagedResourceInstanceObjectMove; cb != nil {
			moveCtx = cb(ctx, addr)
		}

		var moreDiags tfdiags.Diagnostics
		updatedState, moreDiags = resourceType.MoveState(moveCtx, &resources.ManagedResourceMoveStateRequest{
			Provider:      prevRoundState.ProviderInstanceAddr.Config.Config.Provider,
			Resource:      stateInfo.from.Resource.Resource,
			SchemaVersion: prevRoundState.SchemaVersion,
			ValueJSON:     prevRoundState.Value.ValueJSON,
			Private:       prevRoundState.Private,
		}, addr)
		diags = diags.Append(moreDiags)

		if cb := tracer.EndManagedResourceInstanceObjectMove; cb != nil {
			updatedVal := cty.DynamicVal
			if updatedState.Value != cty.NilVal {
				// TODO: Should apply "sensitive" marks here where appropriate in
				// case the tracer is reporting events in the UI.
				updatedVal = updatedState.Value
			}
			cb(moveCtx, addr, updatedVal, diags)
		}
	} else {
		upgradeCtx := ctx
		if cb := tracer.StartManagedResourceInstanceObjectUpgrade; cb != nil {
			upgradeCtx = cb(ctx, addr)
		}

		var moreDiags tfdiags.Diagnostics
		updatedState, moreDiags = resourceType.UpgradeState(upgradeCtx, &resources.ManagedResourceUpgradeStateRequest{
			SchemaVersion: prevRoundState.SchemaVersion,
			ValueJSON:     prevRoundState.Value.ValueJSON,
			Private:       prevRoundState.Private,
		}, addr)
		diags = diags.Append(moreDiags)

		if cb := tracer.EndManagedResourceInstanceObjectUpgrade; cb != nil {
			updatedVal := cty.DynamicVal
			if updatedState.Value != cty.NilVal {
				// TODO: Should apply "sensitive" marks here where appropriate in
				// case the tracer is reporting events in the UI.
				updatedVal = updatedState.Value
			}
			cb(upgradeCtx, addr, updatedVal, diags)
		}
	}
	if diags.HasErrors() {
		return nil, nil, diags
	}

	obj := &states.ResourceInstanceObjectFullSrc{
		Value: states.ValueJSONWithMetadata{
			ValueJSON:      updatedState.ValueJSON,
			SensitivePaths: prevRoundState.Value.SensitivePaths,
		},
		Private:              updatedState.Private,
		Status:               prevRoundState.Status,
		ProviderInstanceAddr: prevRoundState.ProviderInstanceAddr,
		ResourceType:         prevRoundState.ResourceType,
		SchemaVersion:        updatedState.SchemaVersion,
		Dependencies:         prevRoundState.Dependencies,
		ConfigDependencies:   prevRoundState.ConfigDependencies,
		CreateBeforeDestroy:  prevRoundState.CreateBeforeDestroy,
	}

	p.planCtx.upgradedState.SetResourceInstanceObjectFull(writeStateAddr, obj)

	return updatedState, obj, diags
}

func (p *planGlue) planDesiredManagedResourceInstance(
	ctx context.Context,
	inst *eval.DesiredResourceInstance,
) (ret *resourceInstanceObject, diags tfdiags.Diagnostics) {

	configMeta := p.oracle.ResourceInstanceObjectMeta(ctx, inst.Addr.CurrentObject())
	if configMeta == nil {
		// Should not happen: the evaluator is required to always produce
		// non-nil metadata for a desired object.
		panic(fmt.Sprintf("no metadata available for desired object %s", inst.Addr))
	}
	// For a desired object we never pass a prior state object in here because
	// the configuration is expected to be authoritative. We blend configuration
	// and state metadata only for non-desired objects where the configuration
	// tends to be incomplete and so we rely on prior state to fill gaps.
	meta := exec.BuildResourceInstanceObjectMeta(inst.Addr.CurrentObject(), configMeta, (*states.ResourceInstanceObjectFullSrc)(nil))

	// There are various reasons why we might need to defer final planning
	// of this to a later round. The following is not exhaustive but is a
	// placeholder to show where deferral might fit in.
	if p.desiredResourceInstanceMustBeDeferred(inst, meta) {
		// For now, we emulate the original engine, which treats
		// deferral as not-touching the resource at all.
		// See notes below for potential future enhancements.
		return &resourceInstanceObject{
			PlaceholderValue: deferredVal(cty.DynamicVal),
		}, diags
		// We intentionally continue anyway, because we'll make a best effort
		// to produce a speculative plan based on the information we _do_ know
		// in case that allows us to detect a problem sooner. The important
		// thing is that in the deferred case we won't actually propose any
		// planned changes for this resource instance.
		/*defer func() {
			// Our result must be marked as deferred, whichever return path
			// we leave through.
			if ret != nil && ret.PlannedChange != nil && ret.PlannedChange.After != cty.NilVal {
				ret.PlannedChange.After = deferredVal(ret.PlannedChange.After)
			}
			if ret != nil && ret.PlaceholderValue != cty.NilVal {
				ret.PlaceholderValue = deferredVal(ret.PlaceholderValue)
			}
		}()*/
	}

	tracer := contextTracer(ctx)
	if cb := tracer.StartManagedResourceInstanceObjectPlanning; cb != nil {
		ctx = cb(ctx, inst.Addr.CurrentObject())
	}
	if cb := tracer.EndManagedResourceInstanceObjectPlanning; cb != nil {
		defer func() { // closure to delay evaluating diags until we return
			cb(ctx, inst.Addr.CurrentObject(), diags)
		}()
	}

	ret = &resourceInstanceObject{
		Addr:               inst.Addr.CurrentObject(),
		ConfigDependencies: addrs.MakeSet[addrs.AbsResourceInstanceObject](),
		StateDependencies:  addrs.MakeSet[addrs.AbsResourceInstanceObject](),
		Provider:           meta.Provider,

		// We'll start off with a completely-unknown placeholder value, but
		// we might refine this to be more specific as we learn more below.
		PlaceholderValue: cty.DynamicVal,

		// NOTE: PlannedChange remains nil until we actually produce a plan,
		// so early returns with errors are not guaranteed to have a valid
		// change object. Evaluation falls back on using PlaceholderValue
		// when no planned change is present.
	}
	for dep := range inst.RequiredResourceInstances.All() {
		ret.ConfigDependencies.Add(dep.CurrentObject())
	}
	replaceOrder, ok := meta.ReplaceOrder.ValueOk()
	if ok {
		ret.ReplaceOrder = replaceOrder
		if replaceOrder == resources.ReplaceDeleteFirst {
			// TODO: For now the downstream logic only allows for an object
			// to have "create first" ordering or "don't care" ordering, so
			// we'll reject any attempt to force "delete first" ordering at
			// least until we decide how that would be handled by the code
			// that finalizes all of the "don't care" orderings based on
			// any rigid constraints in the same dependency chains.
			// Once we resolve this, we'll need to update either the
			// configgraph package to deny setting "delete first" in the first
			// place or the [findEffectiveReplaceOrders] function to accept
			// that ordering constraint as valid input.
			ret.ReplaceOrder = resources.ReplaceAnyOrder // to allow the remaining planning logic to still complete below
			diags = diags.Append(tfdiags.AttributeValue(
				tfdiags.Error,
				"Unsupported object replacement order",
				// As usual this diagnostic message violates the separation of
				// concerns slightly by talking about surface-level syntax
				// even though we're supposed to be abstracted away from that,
				// but also as usual we'll accept that out of pragmatism.
				"If present, the create_before_destroy argument may be set only to true to force creation-first replacement ordering. Forcing deletion-first is not allowed.",
				nil,
			))
		}
	} else {
		// TODO: Decide what we ought to do in this case. Perhaps we can
		// continue with planning and then treat the change as deferred, but
		// not sure yet if that's actually acceptable since replace order
		// is "infectious" through dependencies and affects the ordering even
		// of non-replace actions between different resource instance objects.
		ret.ReplaceOrder = resources.ReplaceAnyOrder
	}

	providerInstUnmarked, providerInstMarks := meta.ProviderInstance.Unmark()
	// TODO: What should we do with these marks, if anything?
	_ = providerInstMarks
	providerInst, ok := providerInstUnmarked.ValueOk()
	if !ok {
		// If we don't even know which provider instance we're supposed to be
		// talking to then we can't proceed any further.
		return ret, diags
	}

	providerClient, moreDiags := p.providerClient(ctx, providerInst)
	if providerClient == nil {
		moreDiags = moreDiags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Provider instance not available",
			fmt.Sprintf("Cannot plan %s because its associated provider instance %s cannot initialize.", inst.Addr, providerInst),
			nil,
		))
	}
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return ret, diags
	}

	resourceType := resources.NewManagedResourceType(meta.Provider, meta.ResourceType, providerClient)
	schema, schemaDiags := resourceType.LoadSchema(ctx)
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
				inst.Addr, meta.Provider, meta.ResourceType,
			),
			nil, // this error belongs to the whole resource config
		))
		return ret, diags
	}

	// Improve what we know about the potential value
	ret.PlaceholderValue = cty.NullVal(schema.Block.ImpliedType())

	validateDiags := resourceType.ValidateConfig(ctx, inst.ConfigVal)
	diags = diags.Append(validateDiags)
	if diags.HasErrors() {
		return ret, diags
	}

	unmarkedConfigVal, _ := inst.ConfigVal.UnmarkDeep()

	validateDiags = p.planCtx.providers.ValidateResourceConfig(ctx, meta.Provider, addrs.ManagedResourceMode, meta.ResourceType, unmarkedConfigVal)
	diags = diags.Append(validateDiags)
	if diags.HasErrors() {
		return ret, diags
	}

	var prevState *resources.ManagedResourceStateWithIdentity
	var prevStateFull *states.ResourceInstanceObjectFullSrc

	prevStateInfo, moveDiags := p.locateStateForConfig(ctx, inst.Addr)
	diags = diags.Append(moveDiags)
	if diags.HasErrors() {
		return ret, diags
	}

	if prevStateInfo.state != nil {
		for instAddr := range prevStateInfo.state.FlattenedDependencies(p.planCtx.prevRoundState) {
			ret.StateDependencies.Add(instAddr.CurrentObject())
		}
	}

	updatedStateAddr := inst.Addr.CurrentObject()
	if prevStateInfo.isImplicit() {
		// due to a quirk in how moves are handled,
		// if it's an implied move, we save state
		// in the prevAddr instead of current.
		updatedStateAddr = prevStateInfo.from.CurrentObject()
	}

	if prevStateInfo.state != nil {
		var prevValue *resources.EncodedValueWithPrivate
		prevValue, prevStateFull, moreDiags = p.processMoveOrUpgrade(ctx, inst.Addr.CurrentObject(), updatedStateAddr, meta.Provider, resourceType, prevStateInfo)
		diags = diags.Append(moreDiags)
		if diags.HasErrors() {
			return ret, diags
		}
		// TODO Identity passthrough
		prevState = &resources.ManagedResourceStateWithIdentity{EncodedValueWithPrivate: *prevValue}
		ret.PlaceholderValue = prevState.Value
	}

	importing := prevState == nil && !p.planCtx.refreshOnly && configMeta.ImportStatement != nil

	var planImport *plans.Importing
	if importing {
		importTarget := providers.ImportTarget{
			ID:       configMeta.ImportStatement.ID,
			Identity: configMeta.ImportStatement.Identity,
		}

		importCtx := ctx
		if cb := tracer.StartManagedResourceInstanceObjectImport; cb != nil {
			importCtx = cb(ctx, inst.Addr.CurrentObject(), importTarget)
		}

		prevState, moreDiags = resourceType.ImportState(importCtx, &resources.ManagedResourceImportStateRequest{importTarget}, inst.Addr.CurrentObject())
		diags = diags.Append(moreDiags)
		if diags.HasErrors() {
			return ret, diags
		}
		ret.PlaceholderValue = prevState.Value

		if cb := tracer.EndManagedResourceInstanceObjectImport; cb != nil {
			cb(importCtx, inst.Addr.CurrentObject())
		}

		// Fake updatedState for refresh
		prevStateFull = &states.ResourceInstanceObjectFullSrc{
			Value: states.ValueJSONWithMetadata{
				ValueJSON:      prevState.ValueJSON,
				SensitivePaths: nil, // TODO sensitive handling
			},
			Private:              prevState.Private,
			Status:               states.ObjectReady,
			ProviderInstanceAddr: providerInst,
			ResourceType:         inst.Addr.Resource.Resource.Type,
			SchemaVersion:        uint64(schema.Version),
		}

		planImport = &plans.Importing{
			ID:       importTarget.ID,
			Identity: importTarget.Identity,
		}
	}

	if prevState != nil && !p.planCtx.skipRefresh {
		refreshCtx := ctx
		prevValueForTracing := prevState.Value
		if cb := tracer.StartManagedResourceInstanceObjectRefresh; cb != nil {
			refreshCtx = cb(ctx, inst.Addr.CurrentObject(), prevValueForTracing)
		}

		prevState, moreDiags = resourceType.RefreshState(ctx, prevState, inst.Addr.CurrentObject())
		diags = diags.Append(moreDiags)
		if diags.HasErrors() {
			return ret, diags
		}
		ret.PlaceholderValue = prevState.Value

		if cb := tracer.EndManagedResourceInstanceObjectRefresh; cb != nil {
			// TODO: Should apply "sensitive" marks here where appropriate in
			// case the tracer is reporting events in the UI.
			cb(refreshCtx, inst.Addr.CurrentObject(), prevValueForTracing, prevState.Value, diags)
		}

		if !prevState.Value.IsNull() {
			// TODO Identity
			prevStateFull.Value.ValueJSON = prevState.ValueJSON
			prevStateFull.Private = prevState.Private
			// Update the provider instance for the refreshed state only, not the upgraded state
			prevStateFull.ProviderInstanceAddr = providerInst

			// Include config dependencies in prevState
			dependencies := addrs.MakeSet(prevStateFull.Dependencies...)
			configDependencies := addrs.MakeSet(prevStateFull.ConfigDependencies...)
			for dep := range inst.RequiredResourceInstances.All() {
				dependencies.Add(dep)
				configDependencies.Add(dep.ConfigResource())
			}
			prevStateFull.Dependencies = slices.Collect(dependencies.All())
			prevStateFull.ConfigDependencies = slices.Collect(configDependencies.All())

			p.planCtx.refreshedState.SetResourceInstanceObjectFull(updatedStateAddr, prevStateFull)
		} else {
			p.planCtx.refreshedState.RemoveResourceInstanceObjectFull(updatedStateAddr, providerInst)
		}

		// verify the existence of the imported resource
		if importing && prevState.Value.IsNull() {
			var diags tfdiags.Diagnostics
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Cannot import non-existent remote object",
				fmt.Sprintf(
					"While attempting to import an existing object to %q, "+
						"the provider detected that no object exists with the given id or identity. "+
						"Only pre-existing objects can be imported; check that the id or identity "+
						"is correct and that it is associated with the provider's "+
						"configured region or endpoint, or use \"tofu apply\" to "+
						"create a new remote object for this resource.",
					inst.Addr,
				),
			))
			return ret, diags
		}
	}

	// TODO: ProviderMeta is a rarely-used feature that only really makes
	// sense when the module and provider are both written by the same
	// party and the module author is using the provider as a way to
	// transport module usage telemetry. We should decide whether we want
	// to keep supporting that, and if so design a way for the relevant
	// meta value to get from the evaluator into here.
	providerMetaValue := cty.NilVal

	if prevState != nil {
		ret.PlaceholderValue = prevState.Value
	}
	ret.ProviderInst = providerInst

	if p.planCtx.refreshOnly {
		return ret, diags
	}

	var current resources.ValueWithPrivate
	if prevState != nil {
		current = prevState.ValueWithPrivate
	} else {
		// Our best guess as to what the current value is (probably cty.NullVal)
		current.Value = ret.PlaceholderValue
	}

	planChangesCtx := ctx
	if cb := tracer.StartManagedResourceInstanceObjectPlanChanges; cb != nil {
		planChangesCtx = cb(ctx, inst.Addr.CurrentObject(), current.Value, unmarkedConfigVal)
	}
	planResp, planDiags := resourceType.PlanChanges(planChangesCtx, &resources.ManagedResourcePlanRequest{
		Current:            current,
		DesiredValue:       unmarkedConfigVal,
		ProviderMetaValue:  providerMetaValue,
		IgnoreChangesPaths: inst.IgnoreChangesPaths,
	}, ret.Addr)
	diags = diags.Append(planDiags)
	if planDiags.HasErrors() {
		if cb := tracer.EndManagedResourceInstanceObjectPlanChanges; cb != nil {
			cb(planChangesCtx, inst.Addr.CurrentObject(), plans.NoOp, current.Value, cty.DynamicVal, diags)
		}
		return ret, diags
	}

	// Incomplete
	actionReason := plans.ResourceInstanceChangeNoReason

	// Check for if replacement is required
	forceReplace := false
	for _, tb := range inst.ReplaceTriggeredBy {
		replaceAddr, replaceDiags := p.evaluateReplaceTriggeredBy(tb)
		diags = diags.Append(replaceDiags)
		if replaceDiags.HasErrors() {
			return ret, diags
		}

		if replaceAddr != nil {
			log.Printf("[DEBUG] ReplaceTriggeredBy forcing replacement of %s due to change in %s", inst.Addr, replaceAddr)
			forceReplace = true
			actionReason = plans.ResourceInstanceReplaceByTriggers
		}
	}

	// The user might also ask us to force replacing a particular resource
	// instance, regardless of whether the provider thinks it needs replacing.
	// For example, users typically do this if they learn a particular object
	// has become degraded in an immutable infrastructure scenario and so
	// replacing it with a new object is a viable repair path.
	for _, addr := range p.planCtx.forceReplace {
		if addr.Equal(inst.Addr) {
			log.Printf("[DEBUG] Forcing replacement of %s per user request", inst.Addr)
			forceReplace = true
			actionReason = plans.ResourceInstanceReplaceByRequest
		}

		// For "force replace" purposes we require an exact resource instance
		// address to match. If a user forgets to include the instance key
		// for a multi-instance resource then it won't match here, but we
		// have an earlier check in ????? that should
		// prevent us from getting here in that case.
	}

	// TODO: Check for resp.Deferred once we've updated package providers to
	// include it. If that's set then the _provider_ is telling us we must
	// defer planning any action for this resource instance. We'd still
	// return the planned new state as a placeholder for downstream planning in
	// that case, but we would need to mark it as deferred and _not_ record a
	// proposed change for it.

	plannedAction := plans.Update
	if planResp.Current.Value.IsNull() {
		plannedAction = plans.Create
	} else if !planResp.RequiresReplace.Empty() || forceReplace {
		// For "replace" actions the execution graph will include two separate
		// plan and apply operations, where one handles deletion and the other
		// handles creation. There is therefore an implicit third intermediate
		// state between those two, but in our plan model we have a convention
		// to model it as if it were just a direct transition from the old
		// object to the new object.
		//
		// Our current planResp.Planned.Value describes the situation as if
		// we were performing an in-place update though, so we need to now
		// ask the provider to plan each of the parts separately so that we
		// can match how the apply engine will ask the provider these questions.
		createPlanResp, planDiags := resourceType.PlanChanges(ctx, &resources.ManagedResourcePlanRequest{
			// "Current" is intentionally not set here, because we're asking
			// for a plan to create a new object matching the configuration.
			DesiredValue:      unmarkedConfigVal,
			ProviderMetaValue: providerMetaValue,
		}, ret.Addr)
		diags = diags.Append(planDiags)
		if planDiags.HasErrors() {
			return ret, diags
		}
		deletePlanResp, planDiags := resourceType.PlanChanges(ctx, &resources.ManagedResourcePlanRequest{
			Current: current,
			// DesiredValue is intentionally not set here, because we're asking
			// asking for a plan to just destroy what currently exists.
			ProviderMetaValue: providerMetaValue,
		}, ret.Addr)
		diags = diags.Append(planDiags)
		if planDiags.HasErrors() {
			return ret, diags
		}
		// Now we'll update the original plan response with these newly-chosen
		// before/after values, to match what the rest of the system expects.
		planResp.Current = deletePlanResp.Current
		planResp.DesiredValue = createPlanResp.DesiredValue
		planResp.Planned = createPlanResp.Planned

		// We'll select a reasonable initial planned action here but this
		// might be overridden later once we propagate ordering constraints
		// through the dependency graph.
		if replaceOrder == resources.ReplaceCreateFirst {
			plannedAction = plans.CreateThenDelete
		} else {
			plannedAction = plans.DeleteThenCreate
		}
	} else if eq, _ := planResp.Planned.Value.Equals(current.Value).Unmark(); eq.IsKnown() && eq.True() {
		plannedAction = plans.NoOp
	}
	// (a "desired" object cannot have a Delete action; we handle those cases
	// in planOrphanManagedResourceInstance and planDeposedManagedResourceInstanceObject below.)
	ret.PlannedChange = &plans.ResourceInstanceChange{
		Addr:        inst.Addr,
		PrevRunAddr: prevStateInfo.from,
		ProviderAddr: addrs.AbsProviderConfig{
			// FIXME: This is a lossy shim to the old-style provider instance
			// address representation, since our old models aren't yet updated
			// to support the modern one. It cannot handle a provider config
			// inside a module call that uses count or for_each.
			Module:   providerInst.Config.Module.Module(),
			Provider: providerInst.Config.Config.Provider,
			Alias:    providerInst.Config.Config.Alias,
		},
		RequiredReplace: planResp.RequiresReplace,
		Private:         planResp.Planned.Private,
		Action:          plannedAction,
		Before:          planResp.Current.Value,
		After:           planResp.Planned.Value,

		Importing: planImport,

		// TODO: ActionReason, but need to figure out how to get the information
		// we'd need for that into here since most of the reasons are
		// configuration-related and so would need to be driven by stuff in
		// [eval.DesiredResourceInstance].
		ActionReason: actionReason,
	}

	if cb := tracer.EndManagedResourceInstanceObjectPlanChanges; cb != nil {
		plannedVal := cty.DynamicVal
		if planResp.Planned.Value != cty.NilVal {
			// TODO: Should apply "sensitive" marks here where appropriate in
			// case the tracer is reporting events in the UI.
			plannedVal = planResp.Planned.Value
		}
		cb(planChangesCtx, inst.Addr.CurrentObject(), plannedAction, current.Value, plannedVal, diags)
	}

	return ret, diags
}

func (p *planGlue) planOrphanManagedResourceInstance(
	ctx context.Context,
	addr addrs.AbsResourceInstance,
	stateSrc *states.ResourceInstanceObjectFullSrc,
) (*resourceInstanceObject, tfdiags.Diagnostics) {
	return p.planUnwantedManagedResourceInstanceObject(ctx, addr.CurrentObject(), stateSrc)
}

func (p *planGlue) planDeposedManagedResourceInstanceObject(
	ctx context.Context,
	addr addrs.AbsResourceInstanceObject,
	stateSrc *states.ResourceInstanceObjectFullSrc,
) (*resourceInstanceObject, tfdiags.Diagnostics) {
	return p.planUnwantedManagedResourceInstanceObject(ctx, addr, stateSrc)
}

func (p *planGlue) planUnwantedManagedResourceInstanceObject(
	ctx context.Context,
	addr addrs.AbsResourceInstanceObject,
	stateSrc *states.ResourceInstanceObjectFullSrc,
) (*resourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// TODO: This currently has a lot of inline logic that's quite similar to
	// what's in [planGlue.planDesiredManagedResourceInstance]. Once we're
	// satisfied that this set of methods is feature-complete we should consider
	// how to factor out as much of this logic as possible into shared functions
	// so that this'll be easier to maintain in future as requirements change.

	configMeta := p.oracle.ResourceInstanceObjectMeta(ctx, addr)
	meta := exec.BuildResourceInstanceObjectMeta(addr, configMeta, stateSrc)

	ret := &resourceInstanceObject{
		Addr:              addr,
		StateDependencies: addrs.MakeSet[addrs.AbsResourceInstanceObject](),
		Provider:          meta.Provider,

		// Orphan and deposed objects are always planned for deletion, so we can
		// assume the result will be always be some kind of null.
		PlaceholderValue: cty.NullVal(cty.DynamicPseudoType),

		// NOTE: PlannedChange remains nil until we actually produce a plan,
		// so early returns with errors are not guaranteed to have a valid
		// change object. Evaluation falls back on using PlaceholderValue
		// when no planned change is present.
	}
	// TODO: Populate ret.Dependencies based on the dependencies in the state,
	// but to do that we'll need to correlate the [addrs.ConfigResource]-based
	// dependencies with the actual resource instance objects in the prior state
	// to get a comprehensive set of everything we ought to depend on.

	currentRunAddr := addr.InstanceAddr
	movedToAddress := p.locateExecutedMove(currentRunAddr)
	if movedToAddress != nil {
		if addr.IsCurrent() {
			// We were part of a move already and can safely be ignored
			log.Printf("[TRACE] Potentially orphaned resource %s was recorded as being moved to %s and is therefore not orphaned", currentRunAddr, *movedToAddress)
			return ret, diags
		}
	} else {
		movedToAddr, movedDiags := p.locateConfigForState(ctx, currentRunAddr, addr.IsCurrent())
		diags = diags.Append(movedDiags)
		if diags.HasErrors() {
			return ret, diags
		}
		movedToAddress = movedToAddr
	}
	if movedToAddress != nil {
		log.Printf("[TRACE] Orphaned resource %s is moved to %s", currentRunAddr, *movedToAddress)

		// Update the currentRunAddress to match the move
		currentRunAddr = *movedToAddress

		// Discover true configMeta based on state moves
		movedObj := movedToAddress.Object(addr.DeposedKey)
		configMeta = p.oracle.ResourceInstanceObjectMeta(ctx, movedObj)
		meta = exec.BuildResourceInstanceObjectMeta(movedObj, configMeta, stateSrc)

		// Update fields that rely on meta
		ret.Provider = meta.Provider
	}

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
	providerAddr := meta.Provider
	providerInstAddr, ok := meta.ProviderInstance.ValueOk()
	if !ok {
		// TODO: Is there anything sensible to do here? It should only be
		// possible to get into this situation if the unwanted object still
		// has configuration, but we specify the provider instance using a
		// per-resource-instance config setting and so presumably there should
		// never be a provider instance specified in the configuration if a
		// resource instance is "unwanted".
		panic(fmt.Sprintf("unknown provider instance address for %s", providerInstAddr))
	}
	providerClient, moreDiags := p.providerClient(ctx, meta.ProviderInstance.KnownValue())
	if providerClient == nil {
		moreDiags = moreDiags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Provider instance not available",
			fmt.Sprintf("Cannot plan %s because its associated provider instance %s cannot initialize.", addr, providerInstAddr),
			nil,
		))
	}
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return ret, diags
	}

	resourceType := resources.NewManagedResourceType(providerAddr, meta.ResourceType, providerClient)
	schema, schemaDiags := resourceType.LoadSchema(ctx)
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
				addr, providerAddr, meta.ResourceType,
			),
			nil, // this error belongs to the whole resource config
		))
		return ret, diags
	}

	// FIXME: Need to "upgrade" the previous run state before we try to decode
	// it, because the current provider version might be different than the one
	// which most recently updated this object.

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
		return ret, diags
	}
	prevRoundVal = prevRoundState.Value
	prevRoundPrivate = prevRoundState.Private

	for instAddr := range prevRoundState.FlattenedDependencies(p.planCtx.prevRoundState) {
		ret.StateDependencies.Add(instAddr.CurrentObject())
	}

	// Include destroy provisioner dependencies
	for _, p := range meta.PostCreateProvisioners {
		// We intentionally ignore the diagnostics here because we expect they
		// will be collected by the concurrent "CheckAll" walk. We'll just make
		// a best effort to collect whatever dependencies the configuration is
		// valid enough for us to collect.
		cfg, _ := p.BuildConfig(ctx, prevRoundVal)
		for _, ri := range cfg.RequiredResourceInstances {
			ret.ConfigDependencies.Add(ri.CurrentObject())
		}
	}

	// TODO: Call providerClient.ReadResource and update the "refreshed state"
	// and reassign this refreshedVal to the refreshed result.
	refreshedVal := prevRoundVal
	refreshedPrivate := prevRoundPrivate

	if refreshedVal.IsNull() {
		// The orphan object seems to have already been deleted outside of
		// OpenTofu, so we've got nothing more to do here.
		ret.PlaceholderValue = refreshedVal
		return ret, diags
	}

	planResp, planDiags := resourceType.PlanChanges(ctx, &resources.ManagedResourcePlanRequest{
		Current: resources.ValueWithPrivate{
			Value:   refreshedVal,
			Private: refreshedPrivate,
		},
		DesiredValue: cty.NilVal, // we want to destroy this object

		// TODO: ProviderMeta is a rarely-used feature that only really makes
		// sense when the module and provider are both written by the same
		// party and the module author is using the provider as a way to
		// transport module usage telemetry. We should decide whether we want
		// to keep supporting that, and if so design a way for the relevant
		// meta value to get from the evaluator into here.
		ProviderMetaValue: cty.NilVal,
	}, addr)
	diags = diags.Append(planDiags)
	if planDiags.HasErrors() {
		return ret, diags
	}

	ret.PlannedChange = &plans.ResourceInstanceChange{
		Addr:        currentRunAddr,
		PrevRunAddr: addr.InstanceAddr,
		DeposedKey:  addr.DeposedKey,
		ProviderAddr: addrs.AbsProviderConfig{
			// FIXME: This is a lossy shim to the old-style provider instance
			// address representation, since our old models aren't yet updated
			// to support the modern one. It cannot handle a provider config
			// inside a module call that uses count or for_each.
			Module:   providerInstAddr.Config.Module.Module(),
			Provider: providerInstAddr.Config.Config.Provider,
			Alias:    providerInstAddr.Config.Config.Alias,
		},
		RequiredReplace: planResp.RequiresReplace,
		Private:         planResp.Planned.Private,
		Action:          plans.Delete,
		Before:          refreshedVal,
		After:           planResp.Planned.Value,

		// TODO: ActionReason, but need to figure out how to get the information
		// we'd need for that into here. For example, to report that the
		// instance address is no longer in the configuration we need to be
		// able to refer to the configuration in here. Or maybe our caller
		// should just pass in a reason as an additonal argument to this
		// function, since it presumably already knows how it concluded that
		// this address is "orphaned".
	}
	ret.ProviderInst = providerInstAddr
	return ret, diags
}
