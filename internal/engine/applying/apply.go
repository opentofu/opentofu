// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ApplyPlannedChanges is a temporary placeholder entrypoint for a new approach
// to applying based on an execution graph generated during the planning phase.
//
// The signature here is a little confusing because we're currently reusing
// our old-style plan and state models, even though their shape isn't quite
// right for what we need here. A future version of this function will hopefully
// have a signature more tailored to the needs of the new apply engine, once
// we have a stronger understanding of what those needs are.
func ApplyPlannedChanges(ctx context.Context, plan *plans.Plan, configInst *eval.ConfigInstance, plugins plugins.Plugins) (*states.State, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	glue := &evalGlue{
		plugins: plugins,
		// graph field populated below before we actually use the oracle
		// during graph execution.
	}
	oracle, moreDiags := configInst.ApplyOracle(ctx, glue)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}

	execGraph, execCtx, moreDiags := compileExecutionGraph(ctx, plan, oracle, plugins)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}
	glue.graph = execGraph

	moreDiags = execGraph.Execute(ctx)
	diags = diags.Append(moreDiags)

	newState, moreDiags := execCtx.Finish(ctx)
	diags = diags.Append(moreDiags)

	return newState, diags
}

type evalGlue struct {
	graph   *execgraph.CompiledGraph
	plugins plugins.Plugins
}

// ResourceInstanceFinalState implements [eval.ApplyGlue].
func (e *evalGlue) ResourceInstanceFinalState(ctx context.Context, addr addrs.AbsResourceInstance) cty.Value {
	return e.graph.ResourceInstanceValue(ctx, addr)
}

// ValidateProviderConfig implements [eval.ApplyGlue].
func (e *evalGlue) ValidateProviderConfig(ctx context.Context, provider addrs.Provider, configVal cty.Value) tfdiags.Diagnostics {
	return e.plugins.ValidateProviderConfig(ctx, provider, configVal)
}
