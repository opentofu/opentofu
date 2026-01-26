// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
	"log"
	"slices"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/logging"
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

	// Currently we have an odd blend of old and new here as we start to
	// introduce the new "execgraph" concept. So far we're still _mainly_
	// using the plannedChanges field but we're also experimentally populating
	// an execution graph just to learn what's missing in that API in order
	// for us to transition over to it properly.
	plannedChanges   *plans.ChangesSync
	execGraphBuilder *execGraphBuilder

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
	changes := plans.NewChanges()
	refreshedState := prevRoundState.DeepCopy()

	execGraphBuilder := newExecGraphBuilder()

	return &planContext{
		evalCtx:           evalCtx,
		plannedChanges:    changes.SyncWrapper(),
		execGraphBuilder:  execGraphBuilder,
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
func (p *planContext) Close(ctx context.Context) (*plans.Plan, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// We'll freeze the execution graph into a serialized form here, so that
	// we can recover an equivalent execution graph again during the apply
	// phase.
	execGraph := p.execGraphBuilder.Finish()
	if logging.IsDebugOrHigher() {
		log.Println("[DEBUG] Planned execution graph:\n" + logging.Indent(execGraph.DebugRepr()))
	}

	p.closeStackMu.Lock()
	defer p.closeStackMu.Unlock()

	slices.Reverse(p.closeStack)
	for _, closer := range p.closeStack {
		diags = diags.Append(closer(ctx))
	}

	execGraphOpaque := execGraph.Marshal()

	return &plans.Plan{
		UIMode:       plans.NormalMode, // TODO: [PlanChanges] needs something analogous to [tofu.PlanOpts] for planning mode/options
		Changes:      p.plannedChanges.Close(),
		PrevRunState: p.prevRoundState,
		PriorState:   p.refreshedState.Close(),
		// TODO: various other fields that we need to actually make use
		// of this plan result. But this is intentionally just a partial
		// result for now because it's not clear that we'd even be using
		// plans.Plan in a final version of this new approach.

		// This is a special extra field used only by this new runtime,
		// as a probably-temporary place to keep the serialized execution
		// graph so we can round-trip it through saved plan files while
		// the CLI layer is still working in terms of [plans.Plan].
		ExecutionGraph: execGraphOpaque,
	}, diags
}
