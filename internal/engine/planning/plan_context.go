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

	forceReplace []addrs.AbsResourceInstance

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

func newPlanContext(evalCtx *eval.EvalContext, prevRoundState *states.State, providers plugins.Providers, opts *PlanOpts) *planContext {
	if prevRoundState == nil {
		prevRoundState = states.NewState()
	}
	refreshedState := prevRoundState.DeepCopy()
	upgradedState := prevRoundState.DeepCopy()

	return &planContext{
		evalCtx:          evalCtx,
		resourceInstObjs: newResourceInstanceObjectsBuilder(),
		deferred:         addrs.MakeMap[addrs.AbsResourceInstance, struct{}](),
		forceReplace:     opts.ForceReplace,
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

// Handle "prevent_destroy" arguments once the changes have been built,
// generating error diagnostics for anything that is proposed for deletion
// when deletion is prohibited.
func (p *planContextResult) CheckPreventDestroy(ctx context.Context, oracle *eval.PlanningOracle) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	const errSummary = "Invalid value for prevent_destroy"

	for objAddr, obj := range p.ResourceInstanceObjects.All() {
		change := obj.PlannedChange

		if change == nil {
			continue
		}
		if change.Action != plans.Delete && !change.Action.IsReplace() {
			// If we're not attempting to destroy then we'll skip the remaining
			// checks because they are likely to fail dynamically in non-destroy
			// situations even though they could be valid by the time this
			// object actually is planned for destroy.
			continue
		}
		preventDestroyV, rng, pdDiags := oracle.PreventDestroy(ctx, objAddr.InstanceAddr)
		diags = diags.Append(pdDiags)
		if pdDiags.HasErrors() {
			// If PreventDestroy is was specified in an invalid way then we'll
			// assume the diags we just appended already describe the root
			// problem and we'll avoid adding any new errors that might just
			// confusingly restate the same problem in a less direct way.
			continue
		}
		preventDestroyV, preventDestroyMarks := preventDestroyV.Unmark()
		// FIXME: What should we do with these marks, if anything?
		_ = preventDestroyMarks

		preventDestroy, ok := preventDestroyV.ValueOk()
		if !ok {
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
			continue
		}

		if preventDestroy {
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
