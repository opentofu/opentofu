// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"log"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
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
	var instAddr addrs.AbsResourceInstance
	deposedKey := states.NotDeposed
	if desired != nil {
		// By the time we're in the apply phase the desired and prior addresses
		// should already match because the plan phase is responsible for
		// handling concerns like 'moved" blocks that can cause addresses to
		// change, so we'll arbitrarily choose to prefer the desired address
		// whenever both are set.
		instAddr = desired.Addr
		// (deposed objects are never "desired")
	} else if prior != nil {
		instAddr = prior.InstanceAddr
		deposedKey = prior.DeposedKey
	} else {
		// Both should not be nil but if they are then we'll treat it the same
		// way as if we dynamically discover that no change is actually
		// required, by returning a nil final plan to represent "noop".
		log.Printf("[TRACE] applying: ManagedFinalPlan without either desired or prior state, so no change is needed")
		return nil, nil
	}
	if deposedKey == states.NotDeposed {
		log.Printf("[TRACE] applying: ManagedFinalPlan %s using %s", instAddr, providerClient.InstanceAddr)
	} else {
		log.Printf("[TRACE] applying: ManagedFinalPlan %s deposed object %s using %s", instAddr, deposedKey, providerClient.InstanceAddr)
	}
	panic("unimplemented")
}

// ManagedApply implements [exec.Operations].
func (ops *execOperations) ManagedApply(
	ctx context.Context,
	plan *exec.ManagedResourceObjectFinalPlan,
	fallback *exec.ResourceInstanceObject,
	providerClient *exec.ProviderClient,
) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	if plan == nil {
		// TODO: if "fallback" is set then we should set it as current here to
		// honor the overall contract. In practice we currently never construct
		// an execution graph where it's possible for there to be a fallback
		// when there's no plan -- the dynamic absense of a plan is only
		// possible for in-place updates when we learn that no change is
		// actually needed, while fallback is only used for "create then
		// destroy" replacement -- so we'll skip this for now and just do nothing.
		log.Printf("[TRACE] applying: ManagedApply skipped because no change is needed")
		return nil, nil
	}
	if plan.DeposedKey == states.NotDeposed {
		log.Printf("[TRACE] applying: ManagedApply %s using %s", plan.InstanceAddr, providerClient.InstanceAddr)
	} else {
		log.Printf("[TRACE] applying: ManagedApply %s deposed object %s using %s", plan.InstanceAddr, plan.DeposedKey, providerClient.InstanceAddr)
	}

	// This particular operation has a broader scope than most of them because
	// applying changes required careful coordination between the provider
	// calls and the state updates to make sure we always produce a consistent
	// result even in the face of partial failures. We have all of that behavior
	// grouped together into a single operation so that it's easier to read
	// through as normal, linear code without any special control flow, but
	// that comes at the expense of this function doing considerably more
	// work than most other operation methods do.
	panic("unimplemented")
}

// ManagedDepose implements [exec.Operations].
func (ops *execOperations) ManagedDepose(
	ctx context.Context,
	instAddr addrs.AbsResourceInstance,
) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	log.Printf("[TRACE] applying: ManagedDepose %s", instAddr)
	var diags tfdiags.Diagnostics

	deposedKey := ops.workingState.DeposeResourceInstanceObject(instAddr)
	if deposedKey == states.NotDeposed {
		// This means that there was no "current" object to depose, and
		// so we'll return nil to represent that there's nothing here.
		return nil, diags
	}
	return ops.resourceInstanceStateObject(ctx, ops.workingState, instAddr, deposedKey)
}

// ManagedAlreadyDeposed implements [exec.Operations].
func (ops *execOperations) ManagedAlreadyDeposed(
	ctx context.Context,
	instAddr addrs.AbsResourceInstance,
	deposedKey states.DeposedKey,
) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	log.Printf("[TRACE] applying: ManagedAlreadyDeposed %s deposed object %s", instAddr, deposedKey)
	// This is essentially the same as ResourceInstancePrior, but for deposed
	// objects rather than "current" objects. Therefore we'll share most of the
	// implementation between these two.
	return ops.resourceInstanceStateObject(ctx, ops.priorState, instAddr, deposedKey)
}
