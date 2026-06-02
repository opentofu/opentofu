// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"fmt"
	"log"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/resources"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ManagedFinalPlan implements [exec.Operations].
func (ops *execOperations) ManagedFinalPlan(
	ctx context.Context,
	desired *eval.DesiredResourceInstance,
	prior *exec.ResourceInstanceObject,
	initialPlannedVal cty.Value,
) (*exec.ManagedResourceObjectFinalPlan, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	var instAddr addrs.AbsResourceInstance
	var providerConfigAddr addrs.AbsProviderInstanceCorrect
	var resourceTypeName string
	deposedKey := states.NotDeposed
	if desired != nil {
		// By the time we're in the apply phase the desired and prior addresses
		// should already match because the plan phase is responsible for
		// handling concerns like 'moved" blocks that can cause addresses to
		// change, so we'll arbitrarily choose to prefer the desired address
		// whenever both are set.
		instAddr = desired.Addr
		// (deposed objects are never "desired")
		resourceTypeName = desired.ResourceType
		// TODO possibly nil here
		providerConfigAddr = *desired.ProviderInstance
	} else if prior != nil {
		instAddr = prior.Addr.InstanceAddr
		deposedKey = prior.Addr.DeposedKey
		resourceTypeName = prior.State.ResourceType
		providerConfigAddr = prior.State.ProviderInstanceAddr
	} else {
		// Both should not be nil but if they are then we'll treat it the same
		// way as if we dynamically discover that no change is actually
		// required, by returning a nil final plan to represent "noop".
		log.Printf("[TRACE] apply phase: ManagedFinalPlan without either desired or prior state, so no change is needed")
		return nil, diags
	}
	objAddr := instAddr.Object(deposedKey)
	log.Printf("[TRACE] apply phase: ManagedFinalPlan %s using %s", objAddr, providerConfigAddr)

	providerClient, moreDiags := ops.providerInstances.ProviderClient(ctx, providerConfigAddr)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	providerAddr := providerConfigAddr.Config.Config.Provider
	resourceType := resources.NewManagedResourceType(providerAddr, resourceTypeName, providerClient)

	var desiredVal, currentVal cty.Value
	var currentPrivate []byte
	if desired != nil {
		desiredVal, moreDiags = ops.resourceDependenciesMissingCheck("resource", instAddr.String(), desired.ConfigVal)
		diags = diags.Append(moreDiags)
		if moreDiags.HasErrors() {
			return nil, diags
		}
	}
	if prior != nil {
		currentVal = prior.State.Value
		currentPrivate = prior.State.Private
	}

	resp, moreDiags := resourceType.PlanChanges(ctx, &resources.ManagedResourcePlanRequest{
		Current: resources.ValueWithPrivate{
			Value:   currentVal,
			Private: currentPrivate,
		},
		DesiredValue: desiredVal,
		// TODO: Do we want to still support ProviderMeta? If so, who is
		// responsible for propagating its value into here?
		ProviderMetaValue: cty.NilVal,
	}, objAddr)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}

	// The final plan must be a valid concretization of the initial plan,
	// which includes the rule that any known values from the initial plan
	// remain unchanged in the final plan.
	moreDiags = resourceType.ValidateFinalPlan(ctx, initialPlannedVal, resp.Planned.Value, objAddr)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}

	return &exec.ManagedResourceObjectFinalPlan{
		Addr:             instAddr.Object(deposedKey),
		ResourceType:     resourceTypeName,
		PriorStateVal:    resp.Current.Value,
		ConfigVal:        resp.DesiredValue,
		PlannedVal:       resp.Planned.Value,
		ProviderInstance: providerConfigAddr,
		ProviderPrivate:  resp.Planned.Private,
	}, diags
}

// ManagedApply implements [exec.Operations].
func (ops *execOperations) ManagedApply(
	ctx context.Context,
	plan *exec.ManagedResourceObjectFinalPlan,
	fallback *exec.ResourceInstanceObject,
) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	if plan == nil {
		// TODO: if "fallback" is set then we should set it as current here to
		// honor the overall contract. In practice we currently never construct
		// an execution graph where it's possible for there to be a fallback
		// when there's no plan -- the dynamic absense of a plan is only
		// possible for in-place updates when we learn that no change is
		// actually needed, while fallback is only used for "create then
		// destroy" replacement -- so we'll skip this for now and just do nothing.
		log.Printf("[TRACE] apply phase: ManagedApply skipped because no change is needed")
		return nil, diags
	}

	providerConfigAddr := plan.ProviderInstance

	log.Printf("[TRACE] apply phase: ManagedApply %s using %s", plan.Addr, providerConfigAddr)
	if fallback != nil && plan.Addr.IsDeposed() {
		// This should not happen: we can't have a fallback deposed object
		// when the object we're applying is already deposed itself.
		// (This is just a safety check because below we're still using the
		// old states.SyncState API that wants to model the fallback as
		// "maybe restore the deposed object to current" instead of just
		// generically rewriting the fallback object's address to not be deposed.
		diags = diags.Append(fmt.Errorf("can't apply changes to %s with fallback to deposed object %s", plan.Addr, fallback.Addr.DeposedKey))
		return nil, diags
	}

	// This particular operation has a broader scope than most of them because
	// applying changes required careful coordination between the provider
	// calls and the state updates to make sure we always produce a consistent
	// result even in the face of partial failures. We have all of that behavior
	// grouped together into a single operation so that it's easier to read
	// through as normal, linear code without any special control flow, but
	// that comes at the expense of this function doing considerably more
	// work than most other operation methods do.

	providerAddr := providerConfigAddr.Config.Config.Provider
	schema, moreDiags := ops.plugins.ResourceTypeSchema(
		ctx,
		providerAddr,
		addrs.ManagedResourceMode,
		plan.ResourceType,
	)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}

	// TODO: Encapsulate most of the following logic into a method of
	// [resources.ManagedResourceType].

	// TODO: We should preserve the marks from prior and config and reapply
	// them to the result.
	priorValUnmarked, _ := plan.PriorStateVal.UnmarkDeep()
	configValUnmarked, _ := plan.ConfigVal.UnmarkDeep()
	plannedValUnmarked, _ := plan.PlannedVal.UnmarkDeep()

	// Some provider client implementations can't tolerate the values being
	// completely nil, so we'll substitute null values to avoid crashes.
	if priorValUnmarked == cty.NilVal {
		priorValUnmarked = cty.NullVal(schema.Block.ImpliedType())
	}
	if configValUnmarked == cty.NilVal {
		configValUnmarked = cty.NullVal(schema.Block.ImpliedType())
	}
	if plannedValUnmarked == cty.NilVal {
		plannedValUnmarked = cty.NullVal(schema.Block.ImpliedType())
	}

	providerClient, moreDiags := ops.providerInstances.ProviderClient(ctx, providerConfigAddr)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	resp := providerClient.ApplyResourceChange(ctx, providers.ApplyResourceChangeRequest{
		TypeName:       plan.ResourceType,
		PriorState:     priorValUnmarked,
		Config:         configValUnmarked,
		PlannedState:   plannedValUnmarked,
		PlannedPrivate: plan.ProviderPrivate,
		// TODO: Do we want to still support ProviderMeta? If so, who is
		// responsible for propagating its value into here?
		ProviderMeta: cty.NullVal(cty.DynamicPseudoType),
	})
	diags = diags.Append(resp.Diagnostics)
	if resp.NewState == cty.NilVal {
		if !plan.PlannedVal.IsNull() && !diags.HasErrors() {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Provider produced inconsistent result after apply",
				fmt.Sprintf(
					"Provider %s did not return an error when applying changes for %s, but it also didn't return a new object to save.\n\nThis is a bug in the provider, which should be reported in the provider's own issue tracker.",
					providerAddr, plan.Addr,
				),
			))
		}
		// If we were given a "fallback" object then we need to restore it
		// back to being the current object for our resource instance before
		// we return.
		if fallback != nil {
			ok := ops.workingState.MaybeRestoreResourceInstanceDeposed(fallback.Addr.InstanceAddr, fallback.Addr.DeposedKey)
			if !ok {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Failed to restore deposed object",
					fmt.Sprintf(
						"Failed to restore %s deposed object %s as the current object after failing to create its replacement.\n\nThe next plan will propose to destroy this deposed object. This is a bug in OpenTofu.",
						fallback.Addr.InstanceAddr, fallback.Addr.DeposedKey,
					),
				))
			}
		}
		result, moreDiags := ops.resourceInstanceStateObject(ctx, ops.workingState, plan.Addr.InstanceAddr, states.NotDeposed)
		diags = diags.Append(moreDiags)
		return result, diags
	}

	// TODO: objchange.AssertObjectCompatible to verify that the result is
	// consistent with what was planned. (That'll need the provider schema
	// we fetched above, but currently we're just discarding that schema.)

	objAddr := plan.Addr
	var state *states.ResourceInstanceObjectFull
	if !resp.NewState.IsNull() {
		status := states.ObjectTainted
		if !diags.HasErrors() {
			status = states.ObjectReady
		}
		state = &states.ResourceInstanceObjectFull{
			Status:               status,
			Value:                resp.NewState,
			Private:              resp.Private,
			ProviderInstanceAddr: providerConfigAddr,
			ResourceType:         plan.ResourceType,
			SchemaVersion:        uint64(schema.Version),

			// TODO: Propagate the dependencies from the desired object into
			// the final plan and then populate "Dependencies" here.
			// TODO: Propagate whether this resource instance has
			// "create_before_destroy" set into the final plan and then
			// populate CreateBeforeDestroy here.
		}
		stateSrc, err := states.EncodeResourceInstanceObjectFull(state, schema.Block.ImpliedType())
		if err != nil {
			// This is a worst-case scenario where we've successfully changed
			// something but we can't represent what changed in the state for some
			// reason, and so the changes just get lost. It shouldn't be possible
			// to get here in practice though, because resp.NewState would've
			// already been decoded using the same schema if it came from a plugin,
			// and so it should definitely conform to that schema.
			// FIXME: A proper error message for this.
			diags = diags.Append(fmt.Errorf("failed to encode the new state for %s: %w", plan.Addr, err))
			return nil, diags
		}
		ops.workingState.SetResourceInstanceObjectFull(objAddr, stateSrc)
	} else {
		// A null value for "new state" represents that the object has been
		// deleted, so we now just need to remove it from the state.
		// Unfortunately this API is still a little quirkly and wants us to
		// pass the provider instance address so that it can update some
		// resource-level and instance-level metadata as a side-effect.
		ops.workingState.RemoveResourceInstanceObjectFull(objAddr, providerConfigAddr)
	}

	ret := &exec.ResourceInstanceObject{
		Addr:  plan.Addr,
		State: state, // nil if the object was deleted
	}
	return ret, diags
}

// ManagedPerformDepose implements [exec.Operations].
func (ops *execOperations) ManagedPerformDepose(
	ctx context.Context,
	currentObj *exec.ResourceInstanceObject,
	deletePlan *exec.ManagedResourceObjectFinalPlan,
) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	if currentObj == nil {
		log.Println("[TRACE] apply phase: ManagedPerformDepose with nil object (ignored)")
		return nil, diags
	}
	if deletePlan == nil || deletePlan.Addr.IsCurrent() || !deletePlan.PlannedVal.IsNull() || !deletePlan.Addr.InstanceAddr.Equal(currentObj.Addr.InstanceAddr) {
		// None of these situations should arise for a correct execution graph.
		diags = diags.Append(fmt.Errorf(
			"invalid delete plan for %s; this is a bug in OpenTofu",
			currentObj.Addr.InstanceAddr,
		))
		return nil, diags
	}
	log.Printf("[TRACE] apply phase: ManagedPerformDepose %s as %s", currentObj.Addr, deletePlan.Addr.DeposedKey)
	if currentObj.Addr.IsDeposed() {
		diags = diags.Append(fmt.Errorf(
			"attempting do depose %s when it's already deposed; this is a bug in OpenTofu",
			currentObj.Addr,
		))
		return nil, diags
	}

	deposedKey := deletePlan.Addr.DeposedKey
	ops.workingState.DeposeResourceInstanceObjectForceKey(deletePlan.Addr.InstanceAddr, deposedKey)
	return currentObj.IntoDeposed(deposedKey), diags
}

// ManagedAlreadyDeposed implements [exec.Operations].
func (ops *execOperations) ManagedAlreadyDeposed(
	ctx context.Context,
	instAddr addrs.AbsResourceInstance,
	deposedKey states.DeposedKey,
) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	log.Printf("[TRACE] apply phase: ManagedAlreadyDeposed %s deposed object %s", instAddr, deposedKey)
	// This is essentially the same as ResourceInstancePrior, but for deposed
	// objects rather than "current" objects. Therefore we'll share most of the
	// implementation between these two.
	return ops.resourceInstanceStateObject(ctx, ops.priorState, instAddr, deposedKey)
}

// ManagedChangeAddr implements [exec.Operations].
func (ops *execOperations) ManagedChangeAddr(
	ctx context.Context,
	currentObj *exec.ResourceInstanceObject,
	newAddr addrs.AbsResourceInstance,
) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	if currentObj == nil {
		log.Println("[TRACE] apply phase: ManagedChangeAddr with nil object (ignored)")
		return nil, diags
	}
	log.Printf("[TRACE] apply phase: ManagedChangeAddr from %s to %s", currentObj.Addr, newAddr)

	// Only "current" objects are expected to move between addresses in this
	// way, because the only reasonable thing to do with a deposed object is
	// to destroy it.
	if currentObj.Addr.IsDeposed() {
		diags = diags.Append(fmt.Errorf(
			"can't move %s to %s; this is a bug in OpenTofu",
			currentObj.Addr, newAddr,
		))
		return nil, diags
	}

	if !ops.workingState.MaybeMoveResourceInstance(currentObj.Addr.InstanceAddr, newAddr) {
		// We should not get here with a correctly-constructed execution graph
		// because currentObj being non-nil means that there should definitely
		// be something to move.
		diags = diags.Append(fmt.Errorf(
			"failed to move %s to %s; this is a bug in OpenTofu",
			currentObj.Addr, newAddr,
		))
		return nil, diags
	}
	return currentObj.WithNewAddr(newAddr), diags
}
