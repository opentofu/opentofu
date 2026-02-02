package graph

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu/variables"
)

func (c *Context) Eval(ctx context.Context, config *configs.Config, state *states.State, moduleAddr addrs.ModuleInstance, variables variables.InputValues) (*lang.Scope, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	defer c.acquireRun("eval")()

	var walker *ContextGraphWalker

	providerFunctionTracker := make(ProviderFunctionMapping)

	graph, moreDiags := (&EvalGraphBuilder{
		Config:                  config,
		State:                   state,
		RootVariableValues:      variables,
		Plugins:                 c.plugins,
		ProviderFunctionTracker: providerFunctionTracker,
	}).Build(ctx, addrs.RootModuleInstance)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}

	walkOpts := &graphWalkOpts{
		InputState:              state,
		Config:                  config,
		ProviderFunctionTracker: providerFunctionTracker,
	}

	walker, moreDiags = c.walk(ctx, graph, walkEval, walkOpts)
	diags = diags.Append(moreDiags)
	if walker != nil {
		diags = diags.Append(walker.NonFatalDiagnostics)
	} else {
		// If we skipped walking the graph (due to errors) then we'll just
		// use a placeholder graph walker here, which'll refer to the
		// unmodified state.
		walker = c.graphWalker(walkEval, walkOpts)
	}

	// This is a bit weird since we don't normally evaluate outside of
	// the context of a walk, but we'll "re-enter" our desired path here
	// just to get hold of an EvalContext for it. ContextGraphWalker
	// caches its contexts, so we should get hold of the context that was
	// previously used for evaluation here, unless we skipped walking.
	evalCtx := walker.EnterPath(moduleAddr)
	return evalCtx.EvaluationScope(nil, nil, EvalDataForNoInstanceKey), diags
}
