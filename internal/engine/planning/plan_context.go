// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/lang/evalchecks"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// planContext is our shared state for the various parts of a single call
// to [PlanChanges], and is mainly used as part of our [eval.PlanGlue]
// implementation [planGlue], through which the evaluator calls us to ask for
// planning results.
type planContext struct {
	evalCtx *eval.EvalContext

	// resourceInstObjs is where we gradually construct our intermediate
	// representation of the graph of resource instance objects.
	//
	// This gets modified by methods of [planGlue] gradually as we learn of
	// new resource instance objects. Use [planContext.Close] after the
	// work is complete to obtain the finalized object.
	resourceInstObjs *resourceInstanceObjectsBuilder

	// TODO: The following should probably track a reason why each resource
	// instance was deferred, but since deferral is not the focus of this
	// current experiment we'll just keep this boolean for now.
	deferred addrs.Map[addrs.AbsResourceInstance, struct{}]

	// prevRoundState MUST be treated as immutable
	prevRoundState *states.State

	// refreshedState is where we record the results of refreshing
	// resource instances as we visit them. This starts as a deep copy
	// of prevRoundState.
	refreshedState *states.SyncState

	// upgradedState is the state returned by UpgradeResourceState.
	// Each resource instance should modify it once.
	upgradedState *states.SyncState

	// rootOutput is the values and dependencies of the root module outputs
	rootOutput rootOutput

	providers plugins.Providers
}

func newPlanContext(evalCtx *eval.EvalContext, prevRoundState *states.State, providers plugins.Providers) *planContext {
	if prevRoundState == nil {
		prevRoundState = states.NewState()
	}
	refreshedState := prevRoundState.DeepCopy()
	upgradedState := prevRoundState.DeepCopy()

	return &planContext{
		evalCtx:          evalCtx,
		resourceInstObjs: newResourceInstanceObjectsBuilder(),
		deferred:         addrs.MakeMap[addrs.AbsResourceInstance, struct{}](),
		prevRoundState:   prevRoundState,
		refreshedState:   refreshedState.SyncWrapper(),
		upgradedState:    upgradedState.SyncWrapper(),
		providers:        providers,
	}
}

// Close marks the end of the use of the [planContext] object, returning a
// [plans.Plan] representation of the plan that was created.
//
// After calling this function the [planContext] object is invalid and must
// not be used anymore.
func (p *planContext) Close(ctx context.Context) (*planContextResult, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	result := &planContextResult{
		ResourceInstanceObjects: p.resourceInstObjs.Close(),
		PrevRoundState:          p.upgradedState.Close(),
		RefreshedState:          p.refreshedState.Close(),
		RootOutput:              p.rootOutput,
	}

	return result, diags
}

type rootOutput struct {
	Previous map[string]*states.OutputValue
	Current  eval.RootModuleOutputs
}

// planContextResult collects together the intermediate results produced by
// [planContext], ready to be used by the next pass of the planning engine
// to produce the finalized changes and execution graph.
type planContextResult struct {
	ResourceInstanceObjects *resourceInstanceObjects
	PrevRoundState          *states.State
	RefreshedState          *states.State
	RootOutput              rootOutput

	// Unfortunately, we need to signal to the apply engine that some things
	// like output values need to be handled a bit differently.
	Destroying bool
}

// Post-process "prevent_destroy" values now that the changes have been built
//
// This was copied and modified from NodeAbstractResourceInstance.checkPreventDestroy
func (p *planContextResult) CheckPreventDestroy(ctx context.Context, oracle *eval.PlanningOracle) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	const errSummary = "Invalid value for prevent_destroy"

	// Check that prevent_destroy has not been violated
	for objAddr, obj := range p.ResourceInstanceObjects.All() {
		change := obj.PlannedChange

		if change == nil {
			continue
		}
		if change.Action != plans.Delete && !change.Action.IsReplace() {
			// If we're not attempting to destroy then the above checks are
			// sufficient to reject an expression that cannot possibly be valid
			// for prevent_destroy. If we're not actually planning to destroy
			// then we'll skip the remaining checks because they are likely to
			// fail dynamically in non-destroy situations even though they
			// could be valid by the time this object actually is planned for
			// destroy.
			continue
		}
		preventDestroyVal, rng, pdDiags := oracle.PreventDestroy(ctx, objAddr.InstanceAddr)
		if pdDiags.HasErrors() {
			diags = diags.Append(pdDiags)
			continue
		}

		if preventDestroyVal.IsNull() {
			// We could potentially treat null as equivalent to false here, matching
			// how OpenTofu would behave if there were no expression present at all,
			// but "false" is just as easy to specify as "null" in a conditional
			// expression and doesn't require a reader to know what the default
			// is, so we'll require that to make life easier for a future maintainer
			// that isn't necessarily familiar with the prevent_destroy behavior yet.
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  errSummary,
				Detail: fmt.Sprintf(
					"Resource %s has prevent_destroy set to null. When making a dynamic decision to allow destroy, use false instead.",
					objAddr.InstanceAddr,
				),
				Subject: rng.ToHCL().Ptr(),
			})
		}

		if !preventDestroyVal.IsKnown() {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  errSummary,
				Detail: fmt.Sprintf(
					"Resource instance %s has a prevent_destroy argument but its value will not be known until the apply step, so OpenTofu can't predict whether destroying this is acceptable.\n\nTo proceed, exclude instances of this resource from this round using:\n    -exclude=%q",
					objAddr.InstanceAddr.String(), objAddr.InstanceAddr.ContainingResource().String(),
				),
				Subject: rng.ToHCL().Ptr(),
				Extra:   evalchecks.DiagnosticCausedByUnknown(true),
			})
		}
		if preventDestroyVal.IsNull() {
			// We could potentially treat null as equivalent to false here, matching
			// how OpenTofu would behave if there were no expression present at all,
			// but "false" is just as easy to specify as "null" in a conditional
			// expression and doesn't require a reader to know what the default
			// is, so we'll require that to make life easier for a future maintainer
			// that isn't necessarily familiar with the prevent_destroy behavior yet.
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  errSummary,
				Detail: fmt.Sprintf(
					"Resource instance %s has prevent_destroy set to null. When making a dynamic decision to allow destroy, use false instead.",
					objAddr.InstanceAddr.String(),
				),
				Subject: rng.ToHCL().Ptr(),
			})
		}
		if diags.HasErrors() {
			// Any errors so far means that preventDestroyVal.True is likely to
			// either panic or return nonsense.
			continue
		}

		if preventDestroyVal.True() {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Resource instance cannot be destroyed",
				Detail: fmt.Sprintf(
					"Resource instance %s has prevent_destroy set, but the plan calls for it to be destroyed.\n\nTo proceed, either disable prevent_destroy for this resource or exclude instances of this resource from this round using:\n    -exclude=%q",
					objAddr.InstanceAddr.String(), objAddr.InstanceAddr.ContainingResource().String(),
				),
				Subject: rng.ToHCL().Ptr(),
			})
		}
	}
	return diags
}
