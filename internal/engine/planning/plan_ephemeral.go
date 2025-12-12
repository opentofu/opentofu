// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"

	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

func (p *planGlue) planDesiredEphemeralResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance, egb *execgraph.Builder) (cty.Value, execgraph.ResourceInstanceResultRef, tfdiags.Diagnostics) {
	// Regardless of outcome we'll always report that we completed planning.
	defer p.planCtx.reportResourceInstancePlanCompletion(inst.Addr)
	var diags tfdiags.Diagnostics

	validateDiags := p.planCtx.providers.ValidateResourceConfig(ctx, inst.Provider, inst.Addr.Resource.Resource.Mode, inst.Addr.Resource.Resource.Type, inst.ConfigVal)
	diags = diags.Append(validateDiags)
	if diags.HasErrors() {
		return cty.DynamicVal, nil, diags
	}

	// TODO: Implement
	panic("unimplemented")
}
