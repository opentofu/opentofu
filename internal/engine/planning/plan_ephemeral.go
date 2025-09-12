// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"

	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

func (p *planContext) planDesiredEphemeralResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance, oracle *eval.PlanningOracle) (cty.Value, tfdiags.Diagnostics) {
	// TODO: Implement
	panic("unimplemented")
}
