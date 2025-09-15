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

func (p *planContext) planDesiredDataResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance, oracle *eval.PlanningOracle) (cty.Value, tfdiags.Diagnostics) {
	// TODO: Implement
	panic("unimplemented")
}

func (p *planContext) planOrphanDataResourceInstance(ctx context.Context, addr addrs.AbsResourceInstance, state *states.ResourceInstance, oracle *eval.PlanningOracle) tfdiags.Diagnostics {
	// TODO: Implement
	panic("unimplemented")
}
