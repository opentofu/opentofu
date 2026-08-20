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
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/resources"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func (p *planGlue) planDesiredManagedResourceInstance(
	ctx context.Context,
	inst *eval.DesiredResourceInstance,
) (ret *resourceInstanceObject, diags tfdiags.Diagnostics) {

	// There are various reasons why we might need to defer final planning
	// of this to a later round. The following is not exhaustive but is a
	// placeholder to show where deferral might fit in.
	if p.desiredResourceInstanceMustBeDeferred(inst) {
		p.planCtx.deferred.Put(inst.Addr, struct{}{})
		defer func() {
			// Our result must be marked as deferred, whichever return path
			// we leave through.
			if ret != nil && ret.PlannedChange.After != cty.NilVal {
				ret.PlannedChange.After = deferredVal(ret.PlannedChange.After)
			}
		}()
		// We intentionally continue anyway, because we'll make a best effort
		// to produce a speculative plan based on the information we _do_ know
		// in case that allows us to detect a problem sooner. The important
		// thing is that in the deferred case we won't actually propose any
		// planned changes for this resource instance.
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
		Provider:           inst.Provider,

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
	if inst.CreateBeforeDestroy {
		ret.ReplaceOrder = replaceCreateThenDestroy
	}

	if inst.ProviderInstance == nil {
		// If we don't even know which provider instance we're supposed to be
		// talking to then we can't proceed any further.
		return ret, diags
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
		return ret, diags
	}

	resourceType := resources.NewManagedResourceType(inst.Provider, inst.Addr.Resource.Resource.Type, providerClient)
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
				inst.Addr, inst.Provider, inst.Addr.Resource.Resource.Type,
			),
			nil, // this error belongs to the whole resource config
		))
		return ret, diags
	}

	validateDiags := resourceType.ValidateConfig(ctx, inst.ConfigVal)
	diags = diags.Append(validateDiags)
	if diags.HasErrors() {
		return ret, diags
	}

	unmarkedConfigVal, _ := inst.ConfigVal.UnmarkDeep()

	validateDiags = p.planCtx.providers.ValidateResourceConfig(ctx, inst.Provider, inst.ResourceMode, inst.ResourceType, unmarkedConfigVal)
	diags = diags.Append(validateDiags)
	if diags.HasErrors() {
		return ret, diags
	}

	var prevRoundVal cty.Value
	var prevRoundPrivate []byte
	prevRoundState := p.planCtx.prevRoundState.SyncWrapper().ResourceInstanceObjectFull(inst.Addr.CurrentObject())

	prevRunAddr := inst.Addr
	diags = diags.Append(p.oracle.CheckMovesFromAddr(inst.Addr))
	if diags.HasErrors() {
		return ret, diags
	}

	impliedMove := false
	moved := false
	if prevRoundState == nil {
		// check for ambiguous moves
		_, moveDiags := p.oracle.FindAddressesMovedToHere(ctx, inst.Addr)
		diags = diags.Append(moveDiags)
		if diags.HasErrors() {
			// More than one address this could move to.
			// "Ambiguous move statements": many From, one To
			return ret, diags
		}
		if moveInfo, ok := p.oracle.MovedAddress(inst.Addr); ok {
			// we tracked a move in "Unwanted", manage state upgrades and plans here.
			moved = true
			prevRunAddr = moveInfo.From
			prevRoundState = p.planCtx.prevRoundState.SyncWrapper().ResourceInstanceObjectFull(prevRunAddr.CurrentObject())
			impliedMove = moveInfo.Implied
		}
	}
	// only run MoveResourceState or UpgradeResourceState if prevRoundState is non-nil at this point.
	if prevRoundState != nil {
		if moved && !impliedMove && (resourceType.ResourceTypeName() != prevRoundState.ResourceType || !inst.Provider.Equals(prevRoundState.ProviderInstanceAddr.Config.Config.Provider)) {
			moveCtx := ctx
			if cb := tracer.StartManagedResourceInstanceObjectMove; cb != nil {
				moveCtx = cb(ctx, inst.Addr.CurrentObject())
			}

			// log.Printf("[TRACE] moveResourceStateTransform: new address: %s, previous address: %s", inst.Addr, prevRunAddr)
			req := providers.MoveResourceStateRequest{
				SourceProviderAddress: prevRunAddr.Resource.Resource.ImpliedProvider(),
				SourceTypeName:        prevRunAddr.Resource.Resource.Type,
				SourceSchemaVersion:   prevRoundState.SchemaVersion,
				// We'll make the same assumption as [ResourceInstanceObjectFullSrc] and
				// assume we'll never encounter a legacy state snapshot that uses AttrsFlat.
				SourceStateJSON: prevRoundState.Value.ValueJSON,
				// SourceStateFlatmap:    prevRoundState.AttrsFlat,
				SourcePrivate:  prevRoundState.Private,
				TargetTypeName: inst.Addr.Resource.Resource.Type,
			}
			resp := providerClient.MoveResourceState(moveCtx, req)
			diags = diags.Append(resp.Diagnostics)
			// TODO this tracer bit is copypasta for upgrade instance, IDK if it's actually legit...
			if cb := tracer.EndManagedResourceInstanceObjectMove; cb != nil {
				upgradedVal := cty.DynamicVal
				if resp.TargetState != cty.NilVal {
					// TODO: Should apply "sensitive" marks here where appropriate in
					// case the tracer is reporting events in the UI.
					upgradedVal = resp.TargetState
				}
				cb(moveCtx, inst.Addr.CurrentObject(), upgradedVal, diags)
			}
			if diags.HasErrors() {
				return
			}

			src, moreDiags := checkAndMarshalUpdatedState(resp.TargetState, schema, inst)
			diags = diags.Append(moreDiags)
			if diags.HasErrors() {
				return ret, diags
			}

			// TODO this is very similar to what's below in upgraded state.
			// Consider refactoring to de-duplicate
			movedPrevState := &states.ResourceInstanceObjectFullSrc{
				Value: states.ValueJSONWithMetadata{
					ValueJSON:      src,
					SensitivePaths: prevRoundState.Value.SensitivePaths,
				},
				Private:              resp.TargetPrivate,
				Status:               prevRoundState.Status,
				ProviderInstanceAddr: prevRoundState.ProviderInstanceAddr,
				ResourceType:         prevRoundState.ResourceType,
				SchemaVersion:        uint64(schema.Version),
				Dependencies:         prevRoundState.Dependencies,
				CreateBeforeDestroy:  prevRoundState.CreateBeforeDestroy,
			}
			p.planCtx.upgradedState.SetResourceInstanceObjectFull(inst.Addr.CurrentObject(), movedPrevState)
			// Update the provider instance for the refreshed state only, not the "upgraded" state
			movedPrevState.ProviderInstanceAddr = *inst.ProviderInstance
			p.planCtx.refreshedState.SetResourceInstanceObjectFull(inst.Addr.CurrentObject(), movedPrevState)

			obj, err := states.DecodeResourceInstanceObjectFull(movedPrevState, schema.Block.ImpliedType())
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
				return ret, diags
			}
			prevRoundVal = obj.Value
			prevRoundPrivate = resp.TargetPrivate

			if len(prevRoundState.Dependencies) != 0 {
				for _, instAddr := range prevRoundState.Dependencies {
					ret.StateDependencies.Add(instAddr.CurrentObject())
				}
			} else {
				// Unfortunately our old state model represents dependencies only
				// between static [addrs.ConfigResource] and loses specific instance
				// information, so we must conservatively assume that all matching
				// instances are dependencies. This only occurs during migration
				// from a state generated by an older runtime.
				for _, configAddr := range prevRoundState.ConfigDependencies {
					for instAddr := range p.planCtx.prevRoundState.InstancesMatchingConfigResource(configAddr) {
						ret.StateDependencies.Add(instAddr.CurrentObject())
					}
				}
			}
		} else {
			// While we know prevRoundState is non-nil, let's upgrade state, too.
			// Let's do a schema version comparison before upgrade

			if prevRoundState.SchemaVersion > uint64(schema.Version) {
				return ret, diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Resource instance managed by newer provider version",
					// This is not a very good error message, but we don't retain enough
					// information in state to give good feedback on what provider
					// version might be required here. :(
					// Or maybe we do. I dunno, I just copied the comment+diag from
					// upgrade_resource_state.go:upgradeResourceStateTransform :P
					fmt.Sprintf("The current state of %s was created by a newer provider version than is currently selected. Upgrade the %s provider to work with this state.", inst.Addr, inst.Provider.Type),
				))
			}

			upgradeCtx := ctx
			if cb := tracer.StartManagedResourceInstanceObjectUpgrade; cb != nil {
				upgradeCtx = cb(ctx, inst.Addr.CurrentObject())
			}
			upgradeReq := providers.UpgradeResourceStateRequest{
				TypeName: inst.Addr.Resource.Resource.Type,

				// TODO: The internal schema version representations are all using
				// uint64 instead of int64, but unsigned integers aren't friendly
				// to all protobuf target languages so in practice we use int64
				// on the wire. In future we will change all of our internal
				// representations to int64 too.
				Version: int64(prevRoundState.SchemaVersion),

				// We'll make the same assumption as [ResourceInstanceObjectFullSrc] and
				// assume we'll never encounter a legacy state snapshot that uses AttrsFlat.
				RawStateJSON: prevRoundState.Value.ValueJSON,
			}
			upgradeResp := providerClient.UpgradeResourceState(upgradeCtx, upgradeReq)
			diags = diags.Append(upgradeResp.Diagnostics)
			if cb := tracer.EndManagedResourceInstanceObjectUpgrade; cb != nil {
				upgradedVal := cty.DynamicVal
				if upgradeResp.UpgradedState != cty.NilVal {
					// TODO: Should apply "sensitive" marks here where appropriate in
					// case the tracer is reporting events in the UI.
					upgradedVal = upgradeResp.UpgradedState
				}
				cb(upgradeCtx, inst.Addr.CurrentObject(), upgradedVal, diags)
			}
			if diags.HasErrors() {
				return ret, diags
			}
			src, moreDiags := checkAndMarshalUpdatedState(upgradeResp.UpgradedState, schema, inst)
			diags = diags.Append(moreDiags)
			if diags.HasErrors() {
				return ret, diags
			}

			upgradedPrevState := &states.ResourceInstanceObjectFullSrc{
				Value: states.ValueJSONWithMetadata{
					ValueJSON:      src,
					SensitivePaths: prevRoundState.Value.SensitivePaths,
				},
				Private:              prevRoundState.Private,
				Status:               prevRoundState.Status,
				ProviderInstanceAddr: prevRoundState.ProviderInstanceAddr,
				ResourceType:         prevRoundState.ResourceType,
				SchemaVersion:        uint64(schema.Version),
				Dependencies:         prevRoundState.Dependencies,
				ConfigDependencies:   prevRoundState.ConfigDependencies,
				CreateBeforeDestroy:  prevRoundState.CreateBeforeDestroy,
			}

			stateSaveObj := inst.Addr.CurrentObject()
			if impliedMove {
				// due to a quirk in how moves are handled,
				// if it's an implied move, we save state
				// in the prevAddr instead of current.
				stateSaveObj = prevRunAddr.CurrentObject()
			}
			p.planCtx.upgradedState.SetResourceInstanceObjectFull(stateSaveObj, upgradedPrevState)
			// Update the provider instance for the refreshed state only, not the upgraded state
			upgradedPrevState.ProviderInstanceAddr = *inst.ProviderInstance
			p.planCtx.refreshedState.SetResourceInstanceObjectFull(stateSaveObj, upgradedPrevState)

			obj, err := states.DecodeResourceInstanceObjectFull(upgradedPrevState, schema.Block.ImpliedType())
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
				return ret, diags
			}
			prevRoundVal = obj.Value
			prevRoundPrivate = obj.Private

			if len(prevRoundState.Dependencies) != 0 {
				for _, instAddr := range prevRoundState.Dependencies {
					ret.StateDependencies.Add(instAddr.CurrentObject())
				}
			} else {
				// Unfortunately our old state model represents dependencies only
				// between static [addrs.ConfigResource] and loses specific instance
				// information, so we must conservatively assume that all matching
				// instances are dependencies. This only occurs during migration
				// from a state generated by an older runtime.
				for _, configAddr := range prevRoundState.ConfigDependencies {
					for instAddr := range p.planCtx.prevRoundState.InstancesMatchingConfigResource(configAddr) {
						ret.StateDependencies.Add(instAddr.CurrentObject())
					}
				}
			}
		}
	} else {
		// No move or upgrade occurred; this is just a configured address without any state
		// It'll probably get created below
		prevRoundVal = cty.NullVal(schema.Block.ImpliedType())
	}

	// TODO: Call resourceType.RefreshObject, update the "refreshed state",
	// and reassign this refreshedVal to the refreshed result.
	refreshCtx := ctx
	if cb := tracer.StartManagedResourceInstanceObjectRefresh; cb != nil {
		refreshCtx = cb(ctx, inst.Addr.CurrentObject(), prevRoundVal)
	}
	refreshedVal := prevRoundVal
	refreshedPrivate := prevRoundPrivate
	if cb := tracer.EndManagedResourceInstanceObjectRefresh; cb != nil {
		// TODO: Should apply "sensitive" marks here where appropriate in
		// case the tracer is reporting events in the UI.
		cb(refreshCtx, inst.Addr.CurrentObject(), prevRoundVal, refreshedVal, diags)
	}

	// TODO: ProviderMeta is a rarely-used feature that only really makes
	// sense when the module and provider are both written by the same
	// party and the module author is using the provider as a way to
	// transport module usage telemetry. We should decide whether we want
	// to keep supporting that, and if so design a way for the relevant
	// meta value to get from the evaluator into here.
	providerMetaValue := cty.NilVal

	planChangesCtx := ctx
	if cb := tracer.StartManagedResourceInstanceObjectPlanChanges; cb != nil {
		planChangesCtx = cb(ctx, inst.Addr.CurrentObject(), refreshedVal, unmarkedConfigVal)
	}
	planResp, planDiags := resourceType.PlanChanges(planChangesCtx, &resources.ManagedResourcePlanRequest{
		Current: resources.ValueWithPrivate{
			Value:   refreshedVal,
			Private: refreshedPrivate,
		},
		DesiredValue:       unmarkedConfigVal,
		ProviderMetaValue:  providerMetaValue,
		IgnoreChangesPaths: inst.IgnoreChangesPaths,
	}, ret.Addr)
	diags = diags.Append(planDiags)
	if planDiags.HasErrors() {
		if cb := tracer.EndManagedResourceInstanceObjectPlanChanges; cb != nil {
			cb(planChangesCtx, inst.Addr.CurrentObject(), plans.NoOp, refreshedVal, cty.DynamicVal, diags)
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
	if prevRoundState == nil {
		plannedAction = plans.Create
	} else if eq, _ := planResp.Planned.Value.Equals(refreshedVal).Unmark(); eq.IsKnown() && eq.True() && !forceReplace {
		ret.PlaceholderValue = refreshedVal
		plannedAction = plans.NoOp
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
			Current: resources.ValueWithPrivate{
				Value:   refreshedVal,
				Private: refreshedPrivate,
			},
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
		if inst.CreateBeforeDestroy {
			plannedAction = plans.CreateThenDelete
		} else {
			plannedAction = plans.DeleteThenCreate
		}
	}
	// (a "desired" object cannot have a Delete action; we handle those cases
	// in planOrphanManagedResourceInstance and planDeposedManagedResourceInstanceObject below.)
	ret.PlannedChange = &plans.ResourceInstanceChange{
		Addr:        inst.Addr,
		PrevRunAddr: prevRunAddr,
		ProviderAddr: addrs.AbsProviderConfig{
			// FIXME: This is a lossy shim to the old-style provider instance
			// address representation, since our old models aren't yet updated
			// to support the modern one. It cannot handle a provider config
			// inside a module call that uses count or for_each.
			Module:   (*inst.ProviderInstance).Config.Module.Module(),
			Provider: (*inst.ProviderInstance).Config.Config.Provider,
			Alias:    (*inst.ProviderInstance).Config.Config.Alias,
		},
		RequiredReplace: planResp.RequiresReplace,
		Private:         planResp.Planned.Private,
		Change: plans.Change{
			Action: plannedAction,
			Before: planResp.Current.Value,
			After:  planResp.Planned.Value,
		},

		// TODO: ActionReason, but need to figure out how to get the information
		// we'd need for that into here since most of the reasons are
		// configuration-related and so would need to be driven by stuff in
		// [eval.DesiredResourceInstance].
		ActionReason: actionReason,
	}
	ret.ProviderInst = *inst.ProviderInstance

	if cb := tracer.EndManagedResourceInstanceObjectPlanChanges; cb != nil {
		plannedVal := cty.DynamicVal
		if planResp.Planned.Value != cty.NilVal {
			// TODO: Should apply "sensitive" marks here where appropriate in
			// case the tracer is reporting events in the UI.
			plannedVal = planResp.Planned.Value
		}
		cb(planChangesCtx, inst.Addr.CurrentObject(), plannedAction, refreshedVal, plannedVal, diags)
	}

	return ret, diags
}

func checkAndMarshalUpdatedState(newState cty.Value, schema providers.Schema, inst *eval.DesiredResourceInstance) (ret []byte, diags tfdiags.Diagnostics) {
	// After upgrading, the new value must conform to the current schema. When
	// going over RPC this is actually already ensured by the
	// marshaling/unmarshaling of the new value, but we'll check it here
	// anyway for robustness, e.g. for in-process providers.
	if errs := newState.Type().TestConformance(schema.Block.ImpliedType()); len(errs) > 0 {
		providerType := inst.Addr.Resource.Resource.ImpliedProvider()
		for _, err := range errs {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid resource state transformation",
				fmt.Sprintf("The %s provider changed the state for %s, but produced an invalid result: %s.", providerType, inst.Addr, tfdiags.FormatError(err)),
			))
		}
		return nil, diags
	}

	src, err := ctyjson.Marshal(newState, schema.Block.ImpliedType())
	if err != nil {
		// We just checked for type conformance above, so getting into this
		// codepath is probably a bug.
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to encode result of resource state transformation",
			fmt.Sprintf("Failed to encode state for %s after resource schema upgrade: %s.", inst.Addr, tfdiags.FormatError(err)),
		))
	}
	return src, diags
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
	addr addrs.AbsResourceInstance,
	deposedKey states.DeposedKey,
	stateSrc *states.ResourceInstanceObjectFullSrc,
) (*resourceInstanceObject, tfdiags.Diagnostics) {
	return p.planUnwantedManagedResourceInstanceObject(ctx, addr.Object(deposedKey), stateSrc)
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

	ret := &resourceInstanceObject{
		Addr:              addr,
		StateDependencies: addrs.MakeSet[addrs.AbsResourceInstanceObject](),
		Provider:          stateSrc.ProviderInstanceAddr.Config.Config.Provider,

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
	if addr.IsCurrent() {
		// Ask the planning oracle whether there are any "moved" blocks
		// starting at inst.Addr in the configuration (possibly following
		// a chain of multiple moves).
		moveAddrs, moveDiags := p.oracle.FindAddressesMovedFromHere(ctx, addr.InstanceAddr)
		diags = diags.Append(moveDiags)
		if diags.HasErrors() {
			// More than one address this could move to.
			// "Ambiguous move statements": one From, many To
			return ret, diags
		}
		// Check the target instance address of each
		// one in turn in case we find an as-yet-unbound resource address
		// that wants to be rebound to the state given here.
		// Addresses are given from start "From" to final "To"
		foundAddr := false
		blockedMove := false
		for _, nextAddr := range moveAddrs {
			if nextAddr.Equal(addr.InstanceAddr) {
				continue
			}
			if p.oracle.HasAddress(ctx, nextAddr) {
				// found state at a moveable address!
				foundAddr = true
				blockedMove = p.checkStateAndRecordMoveResult(currentRunAddr, nextAddr, addr, false)
				if blockedMove {
					// STOP THE PRESSES!
					// We're going to handle this state with another function call
					// (or the state is already bound to an address).
					// As it is, this state is blocked.

					// We also undo the address, but keep that we "found a move"
					// so we correctly remove this blocked state
					currentRunAddr = addr.InstanceAddr
				} else {
					currentRunAddr = nextAddr
				}
				// If there is another address down the chain, it is an error;
				// you cannot move from an address that exists in configuration.
				// We'll leave the loop now.
				break
			}
		}
		if !foundAddr && !blockedMove {
			// no address found. Try an implicit move
			// TODO a logical question: do we check these implicit moves for EVERY candidate address above?
			// I'd prefer it if we didn't... but I'm afraid that might match what we're expecting...
			// Except! Who in the world is actually combining moves like that???
			implicitMoveAddr, pyrrhicMove := p.oracle.SearchForImplicitMoveableResourceInstance(ctx, addr.InstanceAddr)
			if implicitMoveAddr != nil {
				if pyrrhicMove {
					// We set currentRunAddr to implicitMoveAddr,
					// but we're still going to be deleting it
					// because we didn't actually find an instance
					// in the configuration
					currentRunAddr = *implicitMoveAddr
				} else {
					foundAddr = true
					blockedMove = p.checkStateAndRecordMoveResult(currentRunAddr, *implicitMoveAddr, addr, true)
					if !blockedMove {
						currentRunAddr = *implicitMoveAddr
					}
				}
			}
		}
		if foundAddr && !blockedMove {
			// No change planned: it's moved, and the state movement is handled in "Desired"
			ret.PlannedChange = nil
			return ret, diags
		}
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
	providerAddr := stateSrc.ProviderInstanceAddr.Config.Config.Provider
	providerClient, moreDiags := p.providerClient(ctx, stateSrc.ProviderInstanceAddr)
	if providerClient == nil {
		moreDiags = moreDiags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Provider instance not available",
			fmt.Sprintf("Cannot plan %s because its associated provider instance %s cannot initialize.", addr, stateSrc.ProviderInstanceAddr),
			nil,
		))
	}
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return ret, diags
	}

	resourceType := resources.NewManagedResourceType(providerAddr, addr.InstanceAddr.Resource.Resource.Type, providerClient)
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
				addr, providerAddr, addr.InstanceAddr.Resource.Resource.Type,
			),
			nil, // this error belongs to the whole resource config
		))
		return ret, diags
	}

	// FIXME: Need to "upgrade" the previous run state and then refresh it
	// before we try to decode it.

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

	if len(prevRoundState.Dependencies) != 0 {
		for _, instAddr := range prevRoundState.Dependencies {
			ret.StateDependencies.Add(instAddr.CurrentObject())
		}
	} else {
		// Unfortunately our old state model represents dependencies only
		// between static [addrs.ConfigResource] and loses specific instance
		// information, so we must conservatively assume that all matching
		// instances are dependencies. This only occurs during migration
		// from a state generated by an older runtime.
		for _, configAddr := range prevRoundState.ConfigDependencies {
			for instAddr := range p.planCtx.prevRoundState.InstancesMatchingConfigResource(configAddr) {
				ret.StateDependencies.Add(instAddr.CurrentObject())
			}
		}
	}

	// Include destroy provisioner dependencies
	// FIXME: Use the resource instance object metadata API to do this, which
	// means extending [evalglue.ResourceProvisionerConfig] to include a set
	// of resource instances required for each provisioner configuration.
	// (Intentionally leaving this unresolved for now because the work to
	// implement "moved" blocks is likely to cause significant changes to how
	// we handle "unwanted" resource instances, so don't want to change the
	// flow of things here too much right now.)
	/*
		for _, prov := range p.oracle.DestroyProvisioners(ctx, addr.InstanceAddr) {
			for ri := range prov.Dependencies {
				ret.ConfigDependencies.Add(ri.Addr.CurrentObject())
			}
		}
	*/

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
			Module:   prevRoundState.ProviderInstanceAddr.Config.Module.Module(),
			Provider: prevRoundState.ProviderInstanceAddr.Config.Config.Provider,
			Alias:    prevRoundState.ProviderInstanceAddr.Config.Config.Alias,
		},
		RequiredReplace: planResp.RequiresReplace,
		Private:         planResp.Planned.Private,
		Change: plans.Change{
			Action: plans.Delete,
			Before: refreshedVal,
			After:  planResp.Planned.Value,
		},

		// TODO: ActionReason, but need to figure out how to get the information
		// we'd need for that into here. For example, to report that the
		// instance address is no longer in the configuration we need to be
		// able to refer to the configuration in here. Or maybe our caller
		// should just pass in a reason as an additonal argument to this
		// function, since it presumably already knows how it concluded that
		// this address is "orphaned".
	}
	ret.ProviderInst = prevRoundState.ProviderInstanceAddr
	return ret, diags
}

// checkStateAndRecordMoveResult returns true is a successful move and false if
// the move is blocked by pre-existing state
func (p *planGlue) checkStateAndRecordMoveResult(currentRunAddr addrs.AbsResourceInstance, nextAddr addrs.AbsResourceInstance, addr addrs.AbsResourceInstanceObject, impliedMove bool) bool {
	preExistingState := p.planCtx.prevRoundState.SyncWrapper().ResourceInstanceObjectFull(nextAddr.CurrentObject())
	if preExistingState != nil {
		p.oracle.RecordBlockedMove(currentRunAddr, nextAddr)
		return true
	}
	// Record move in the oracle, for use in "Desired" section
	p.oracle.RecordSuccessfulMove(nextAddr, addr.InstanceAddr, impliedMove)
	return false
}
