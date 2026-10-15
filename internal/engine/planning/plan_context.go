// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
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

	// TODO once deferred is fully functional, we might be able to use that
	// combined with resourceInstanceObjs to replace these fields
	desiredMu sync.Mutex
	desired   addrs.Set[addrs.AbsResourceInstance]

	forceReplace []addrs.AbsResourceInstance

	moveMu          sync.Mutex
	recordedMoves   addrs.Map[addrs.AbsResourceInstance, addrs.AbsResourceInstance]
	configuredMoves addrs.Map[addrs.AbsResourceInstance, []*moveStep]

	// prevRoundState MUST be treated as immutable
	prevRoundState *states.State

	// rootOutput is the values and dependencies of the root module outputs
	rootOutput rootOutput

	providers plugins.Providers
}

func newPlanContext(evalCtx *eval.EvalContext, prevRoundState *states.State, providers plugins.Providers, opts *PlanOpts) *planContext {
	if prevRoundState == nil {
		prevRoundState = states.NewState()
	}
	return &planContext{
		evalCtx:          evalCtx,
		resourceInstObjs: newResourceInstanceObjectsBuilder(),
		deferred:         addrs.MakeMap[addrs.AbsResourceInstance, struct{}](),
		desired:          addrs.MakeSet[addrs.AbsResourceInstance](),
		forceReplace:     opts.ForceReplace,
		recordedMoves:    addrs.MakeMap[addrs.AbsResourceInstance, addrs.AbsResourceInstance](),
		configuredMoves:  addrs.MakeMap[addrs.AbsResourceInstance, []*moveStep](),
		prevRoundState:   prevRoundState,
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
		PrevRoundState:          p.prevRoundState,
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
	RootOutput              rootOutput

	// Unfortunately, we need to signal to the apply engine that some things
	// like output values need to be handled a bit differently.
	Destroying bool
}
