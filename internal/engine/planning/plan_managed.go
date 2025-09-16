// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func (p *planGlue) planDesiredManagedResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance) (cty.Value, tfdiags.Diagnostics) {
	// Regardless of outcome we'll always report that we completed planning.
	defer p.planCtx.reportResourceInstancePlanCompletion(inst.Addr)

	// TODO: Implement
	panic("unimplemented")
}

func (p *planGlue) planOrphanManagedResourceInstance(ctx context.Context, addr addrs.AbsResourceInstance, state *states.ResourceInstance) tfdiags.Diagnostics {
	// Regardless of outcome we'll always report that we completed planning.
	defer p.planCtx.reportResourceInstancePlanCompletion(addr)

	// TODO: Implement
	panic("unimplemented")
}

func (p *planGlue) planDeposedManagedResourceInstanceObject(ctx context.Context, addr addrs.AbsResourceInstance, deposedKey states.DeposedKey, state *states.ResourceInstance) tfdiags.Diagnostics {
	// Regardless of outcome we'll always report that we completed planning.
	defer p.planCtx.reportResourceInstanceDeposedPlanCompletion(addr, deposedKey)

	// TODO: Implement
	panic("unimplemented")
}
