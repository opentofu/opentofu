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
	providerClient *exec.ProviderClient,
) (*exec.ManagedResourceObjectFinalPlan, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	var instAddr addrs.AbsResourceInstance
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
	} else if prior != nil {
		instAddr = prior.InstanceAddr
		deposedKey = prior.DeposedKey
		resourceTypeName = prior.State.ResourceType
	} else {
		// Both should not be nil but if they are then we'll treat it the same
		// way as if we dynamically discover that no change is actually
		// required, by returning a nil final plan to represent "noop".
		log.Printf("[TRACE] apply phase: ManagedFinalPlan without either desired or prior state, so no change is needed")
		return nil, diags
	}
	objAddr := instAddr.Object(deposedKey)
	log.Printf("[TRACE] apply phase: ManagedFinalPlan %s using %s", objAddr, providerClient.InstanceAddr)

	providerAddr := providerClient.InstanceAddr.Config.Config.Provider
	resourceType := resources.NewManagedResourceType(providerAddr, resourceTypeName, providerClient.Ops)

	var desiredVal, currentVal cty.Value
	var currentPrivate []byte
	if desired != nil {
		desiredVal = desired.ConfigVal
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
		InstanceAddr:    instAddr,
		DeposedKey:      deposedKey,
		ResourceType:    resourceTypeName,
		PriorStateVal:   resp.Current.Value,
		ConfigVal:       resp.DesiredValue,
		PlannedVal:      resp.Planned.Value,
		ProviderPrivate: resp.Planned.Private,
	}, diags
}

// ManagedApply implements [exec.Operations].
func (ops *execOperations) ManagedApply(
	ctx context.Context,
	plan *exec.ManagedResourceObjectFinalPlan,
	fallback *exec.ResourceInstanceObject,
	providerClient *exec.ProviderClient,
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
	if plan.DeposedKey == states.NotDeposed {
		log.Printf("[TRACE] apply phase: ManagedApply %s using %s", plan.InstanceAddr, providerClient.InstanceAddr)
	} else {
		log.Printf("[TRACE] apply phase: ManagedApply %s deposed object %s using %s", plan.InstanceAddr, plan.DeposedKey, providerClient.InstanceAddr)
	}
	if fallback != nil && plan.DeposedKey != states.NotDeposed {
		// This should not happen: we can't have a fallback deposed object
		// when the object we're applying is already deposed itself.
		// (This is just a safety check because below we're still using the
		// old states.SyncState API that wants to model the fallback as
		// "maybe restore the deposed object to current" instead of just
		// generically rewriting the fallback object's address to not be deposed.
		diags = diags.Append(fmt.Errorf("can't apply changes to %s deposed object %s with fallback to deposed object %s", plan.InstanceAddr, plan.DeposedKey, fallback.DeposedKey))
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

	providerAddr := providerClient.InstanceAddr.Config.Config.Provider
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

	resp := providerClient.Ops.ApplyResourceChange(ctx, providers.ApplyResourceChangeRequest{
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
					providerAddr, plan.InstanceAddr,
				),
			))
		}
		// If we were given a "fallback" object then we need to restore it
		// back to being the current object for our resource instance before
		// we return.
		ok := ops.workingState.MaybeRestoreResourceInstanceDeposed(fallback.InstanceAddr, fallback.DeposedKey)
		if !ok {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Failed to restore deposed object",
				fmt.Sprintf(
					"Failed to restore %s deposed object %s as the current object after failing to create its replacement.\n\nThe next plan will propose to destroy this deposed object. This is a bug in OpenTofu.",
					fallback.InstanceAddr, fallback.DeposedKey,
				),
			))
		}
		result, moreDiags := ops.resourceInstanceStateObject(ctx, ops.workingState, plan.InstanceAddr, states.NotDeposed)
		diags = diags.Append(moreDiags)
		return result, diags
	}

	// TODO: objchange.AssertObjectCompatible to verify that the result is
	// consistent with what was planned. (That'll need the provider schema
	// we fetched above, but currently we're just discarding that schema.)

	// FIXME: Change [exec.ManagedResourceObjectFinalPlan] to use
	// [addrs.AbsResourceInstanceObject] itself, instead of separate instance
	// address and deposed key fields.
	objAddr := plan.InstanceAddr.Object(plan.DeposedKey)
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
			ProviderInstanceAddr: providerClient.InstanceAddr,
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
			diags = diags.Append(fmt.Errorf("failed to encode the new state for %s: %w", plan.InstanceAddr, err))
			return nil, diags
		}
		ops.workingState.SetResourceInstanceObjectFull(objAddr, stateSrc)
	} else {
		// A null value for "new state" represents that the object has been
		// deleted, so we now just need to remove it from the state.
		// Unfortunately this API is still a little quirkly and wants us to
		// pass the provider instance address so that it can update some
		// resource-level and instance-level metadata as a side-effect.
		ops.workingState.RemoveResourceInstanceObjectFull(objAddr, providerClient.InstanceAddr)
	}

	ret := &exec.ResourceInstanceObject{
		InstanceAddr: plan.InstanceAddr,
		DeposedKey:   plan.DeposedKey,
		State:        state, // nil if the object was deleted
	}
	return ret, diags
}

// ManagedDepose implements [exec.Operations].
func (ops *execOperations) ManagedDepose(
	ctx context.Context,
	currentObj *exec.ResourceInstanceObject,
) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	if currentObj == nil {
		log.Println("[TRACE] apply phase: ManagedDepose with nil object (ignored)")
		return nil, diags
	}
	log.Printf("[TRACE] apply phase: ManagedDepose %s", currentObj.InstanceAddr)

	deposedKey := ops.workingState.DeposeResourceInstanceObject(currentObj.InstanceAddr)
	if deposedKey == states.NotDeposed {
		// We should not get here with a correctly-constructed execution graph
		// because currentObj being non-nil means that there should definitely
		// be something to depose.
		diags = diags.Append(fmt.Errorf(
			"failed to depose the current object for %s; this is a bug in OpenTofu",
			currentObj.InstanceAddr,
		))
		return nil, diags
	}
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
	log.Printf("[TRACE] apply phase: ManagedChangeAddr from %s to %s", currentObj.InstanceAddr, newAddr)
	if !ops.workingState.MaybeMoveResourceInstance(currentObj.InstanceAddr, newAddr) {
		// We should not get here with a correctly-constructed execution graph
		// because currentObj being non-nil means that there should definitely
		// be something to move.
		diags = diags.Append(fmt.Errorf(
			"failed to move %s to %s; this is a bug in OpenTofu",
			currentObj.InstanceAddr, newAddr,
		))
		return nil, diags
	}
	return currentObj.WithNewAddr(newAddr), diags
}
