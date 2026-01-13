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
func ApplyPlannedChanges(ctx context.Context, plan *plans.Plan, configInst *eval.ConfigInstance, providers plugins.Providers) (*states.State, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	diags = diags.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"New apply engine not yet implemented",
		"The new-style apply engine is not yet implemented.",
	))
	return nil, diags
}
