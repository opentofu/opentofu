// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/lifecycle"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
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

	completion *completionTracker

	providerInstances *providerInstances

	providers plugins.Providers

	// TODO: something to track which ephemeral resource instances are currently
	// open? (Do we actually need that, or can we just rely on a background
	// goroutine to babysit those based on the completion tracker?)
}

func newPlanContext(evalCtx *eval.EvalContext, prevRoundState *states.State, providers plugins.Providers) *planContext {
	if prevRoundState == nil {
		prevRoundState = states.NewState()
	}
	changes := plans.NewChanges()
	refreshedState := prevRoundState.DeepCopy()

	completion := lifecycle.NewCompletionTracker[completionEvent]()

	execGraphBuilder := newExecGraphBuilder()

	return &planContext{
		evalCtx:           evalCtx,
		plannedChanges:    changes.SyncWrapper(),
		execGraphBuilder:  execGraphBuilder,
		prevRoundState:    prevRoundState,
		refreshedState:    refreshedState.SyncWrapper(),
		completion:        completion,
		providerInstances: newProviderInstances(completion),
		providers:         providers,
	}
}

// Close marks the end of the use of the [planContext] object, returning a
// [plans.Plan] representation of the plan that was created.
//
// After calling this function the [planContext] object is invalid and must
// not be used anymore.
func (p *planContext) Close() *plans.Plan {
	// Before we return we'll make sure our completion tracker isn't waiting
	// for anything else to complete, so that we can unblock closing of
	// any provider instances or ephemeral resource instances that might've
	// got left behind by panics/etc. We should not be relying on this in the
	// happy path.
	for event := range p.completion.PendingItems() {
		log.Printf("[TRACE] planContext: synthetic completion of %#v", event)
		p.completion.ReportCompletion(event)
	}

	// We'll freeze the execution graph into a serialized form here, so that
	// we can recover an equivalent execution graph again during the apply
	// phase.
	execGraph := p.execGraphBuilder.Finish()
	if logging.IsDebugOrHigher() {
		log.Println("[DEBUG] Planned execution graph:\n" + logging.Indent(execGraph.DebugRepr()))
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
	}
}
