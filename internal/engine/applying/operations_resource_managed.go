// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plans/objchange"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ManagedFinalPlan implements [exec.Operations].
func (ops *execOperations) ManagedFinalPlan(
	ctx context.Context,
	desired *eval.DesiredResourceInstance,
	prior *exec.ResourceInstanceObject,
	plannedVal cty.Value,
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
	if deposedKey == states.NotDeposed {
		log.Printf("[TRACE] apply phase: ManagedFinalPlan %s using %s", instAddr, providerClient.InstanceAddr)
	} else {
		log.Printf("[TRACE] apply phase: ManagedFinalPlan %s deposed object %s using %s", instAddr, deposedKey, providerClient.InstanceAddr)
	}

	// TODO: Find a good place to centralize a function for asking a provider
	// to produce a plan, which we can then share between this operation and
	// the equivalent step in the planning engine. But we'll need to figure
	// out how best to frame that shared operation because the planning engine
	// has the additional need of recognizing whether an "update" operation
	// needs to be treated as a "replace", whereas the execution graph should
	// already have "replace" actions decomposed into separate create and
	// destroy actions.
	//
	// For now we just have a simple implementation inline, which is good
	// enough as a proof-of-concept.

	providerAddr := providerClient.InstanceAddr.Config.Config.Provider
	schema, moreDiags := ops.plugins.ResourceTypeSchema(
		ctx,
		providerAddr,
		addrs.ManagedResourceMode,
		resourceTypeName,
	)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}

	var configVal, priorVal cty.Value
	var priorPrivate []byte
	if desired != nil {
		configVal = desired.ConfigVal
	} else {
		configVal = cty.NullVal(cty.DynamicPseudoType)
	}
	if prior != nil {
		priorVal = prior.State.Value
		priorPrivate = prior.State.Private
	} else {
		priorVal = cty.NullVal(cty.DynamicPseudoType)
	}
	proposedVal := objchange.ProposedNew(schema.Block, priorVal, configVal)

	// TODO: We should preserve the marks from prior and config and reapply
	// them to the result.
	priorValUnmarked, _ := priorVal.UnmarkDeep()
	configValUnmarked, _ := configVal.UnmarkDeep()
	proposedValUnmarked, _ := proposedVal.UnmarkDeep()

	resp := providerClient.Ops.PlanResourceChange(ctx, providers.PlanResourceChangeRequest{
		TypeName:         resourceTypeName,
		PriorState:       priorValUnmarked,
		Config:           configValUnmarked,
		ProposedNewState: proposedValUnmarked,
		PriorPrivate:     priorPrivate,
		// TODO: Do we want to still support ProviderMeta? If so, who is
		// responsible for propagating its value into here?
		ProviderMeta: cty.NullVal(cty.DynamicPseudoType),
	})
	diags = diags.Append(resp.Diagnostics)
	if resp.Diagnostics.HasErrors() {
		return nil, diags
	}

	if errs := objchange.AssertPlanValid(schema.Block, priorValUnmarked, configValUnmarked, plannedVal); len(errs) > 0 {
		if resp.LegacyTypeSystem {
			// The shimming of the old type system in the legacy SDK is not precise
			// enough to pass this consistency check, so we'll give it a pass here,
			// but we will generate a warning about it so that we are more likely
			// to notice in the logs if an inconsistency beyond the type system
			// leads to a downstream provider failure.
			var buf strings.Builder
			fmt.Fprintf(&buf,
				"[WARN] Provider %q produced an invalid plan for %s, but we are tolerating it because it is using the legacy plugin SDK.\n    The following problems may be the cause of any confusing errors from downstream operations:",
				providerAddr, instAddr,
			)
			for _, err := range errs {
				fmt.Fprintf(&buf, "\n      - %s", tfdiags.FormatError(err))
			}
			log.Print(buf.String())
		} else {
			for _, err := range errs {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Provider produced invalid plan",
					fmt.Sprintf(
						"Provider %q planned an invalid value for %s.\n\nThis is a bug in the provider, which should be reported in the provider's own issue tracker.",
						providerAddr, tfdiags.FormatErrorPrefixed(err, instAddr.String()),
					),
				))
			}
			return nil, diags
		}
	}

	return &exec.ManagedResourceObjectFinalPlan{
		InstanceAddr:    instAddr,
		DeposedKey:      deposedKey,
		ResourceType:    resourceTypeName,
		PriorStateVal:   priorVal,
		ConfigVal:       configVal,
		PlannedVal:      resp.PlannedState,
		ProviderPrivate: resp.PlannedPrivate,
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

	// TODO: We should preserve the marks from prior and config and reapply
	// them to the result.
	priorValUnmarked, _ := plan.PriorStateVal.UnmarkDeep()
	configValUnmarked, _ := plan.ConfigVal.UnmarkDeep()
	plannedValUnmarked, _ := plan.PlannedVal.UnmarkDeep()

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

	status := states.ObjectTainted
	if !diags.HasErrors() {
		status = states.ObjectReady
	}
	ret := &exec.ResourceInstanceObject{
		InstanceAddr: plan.InstanceAddr,
		DeposedKey:   plan.DeposedKey,
		State: &states.ResourceInstanceObjectFull{
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
		},
	}
	stateSrc, err := states.EncodeResourceInstanceObjectFull(ret.State, schema.Block.ImpliedType())
	if err != nil {
		// This is a worst-case scenario where we've successfully changed
		// something but we can't represent what changed in the state for some
		// reason, and so the changes just get lost. It shouldn't be possible
		// to get here in practice though, because resp.NewState would've
		// already been decoded using the same schema if it came from a plugin,
		// and so it should definitely conform to that schema.
		// FIXME: A proper error message for this.
		diags = diags.Append(fmt.Errorf("failed to encode the new state for %s: %w", plan.InstanceAddr, err))
		return ret, diags
	}
	ops.workingState.SetResourceInstanceObjectFull(plan.InstanceAddr, plan.DeposedKey, stateSrc)
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
