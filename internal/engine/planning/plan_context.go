// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
	"slices"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/lang/eval"
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

	providers plugins.Providers

	providerInstances *providerInstances

	// Stack of ephemeral and provider close functions
	// Given the current state of the planning engine, we wait until
	// the end of the run to close all of the "opened" items.  We
	// also need to close them in a specific order to prevent dependency
	// conflicts. We posit that for plan, closing in the reverse order of opens
	// will ensure that this order is correctly preserved.
	closeStackMu sync.Mutex
	closeStack   []func(context.Context) tfdiags.Diagnostics
}

func newPlanContext(evalCtx *eval.EvalContext, prevRoundState *states.State, providers plugins.Providers) *planContext {
	if prevRoundState == nil {
		prevRoundState = states.NewState()
	}
	refreshedState := prevRoundState.DeepCopy()

	return &planContext{
		evalCtx:           evalCtx,
		resourceInstObjs:  newResourceInstanceObjectsBuilder(),
		prevRoundState:    prevRoundState,
		refreshedState:    refreshedState.SyncWrapper(),
		providerInstances: newProviderInstances(),
		providers:         providers,
	}
}

// Close marks the end of the use of the [planContext] object, returning a
// [plans.Plan] representation of the plan that was created.
//
// After calling this function the [planContext] object is invalid and must
// not be used anymore.
func (p *planContext) Close(ctx context.Context) (*planContextResult, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	p.closeStackMu.Lock()
	slices.Reverse(p.closeStack)
	for _, closer := range p.closeStack {
		diags = diags.Append(closer(ctx))
	}
	p.closeStackMu.Unlock()

	return &planContextResult{
		ResourceInstanceObjects: p.resourceInstObjs.Close(),
		PrevRoundState:          p.prevRoundState,
		RefreshedState:          p.refreshedState.Close(),
	}, diags
}

// planContextResult collects together the intermediate results produced by
// [planContext], ready to be used by the next pass of the planning engine
// to produce the finalized changes and execution graph.
type planContextResult struct {
	ResourceInstanceObjects *resourceInstanceObjects
	PrevRoundState          *states.State
	RefreshedState          *states.State
}
