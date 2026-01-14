// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"

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

	execGraph, execCtx, moreDiags := compileExecutionGraph(ctx, plan, plugins)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}

	moreDiags = execGraph.Execute(ctx)
	diags = diags.Append(moreDiags)

	newState, moreDiags := execCtx.Finish(ctx)
	diags = diags.Append(moreDiags)

	return newState, diags
}
