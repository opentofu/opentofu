// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
	"fmt"

	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/shared"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

func (p *planGlue) planDesiredEphemeralResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance, egb *execGraphBuilder) (cty.Value, execgraph.ResourceInstanceResultRef, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	schema, _ := p.planCtx.providers.ResourceTypeSchema(ctx, inst.Provider, inst.Addr.Resource.Resource.Mode, inst.Addr.Resource.Resource.Type)
	if schema == nil || schema.Block == nil {
		// Should be caught during validation, so we don't bother with a pretty error here
		diags = diags.Append(fmt.Errorf("provider %q does not support ephemeral resource %q", inst.ProviderInstance, inst.Addr.Resource.Resource.Type))
		return cty.NilVal, nil, diags
	}

	if inst.ProviderInstance == nil {
		// If we don't even know which provider instance we're supposed to be
		// talking to then we'll just return a placeholder value, because
		// we don't have any way to generate a speculative plan.
		return cty.NilVal, nil, diags
	}

	providerClient, moreDiags := p.providerClient(ctx, *inst.ProviderInstance)
	if providerClient == nil {
		moreDiags = moreDiags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Provider instance not available",
			fmt.Sprintf("Cannot plan %s because its associated provider instance %s cannot initialize.", inst.Addr, *inst.ProviderInstance),
			nil,
		))
	}
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return cty.NilVal, nil, diags
	}

	newVal, closeFunc, openDiags := shared.OpenEphemeralResourceInstance(
		ctx,
		inst.Addr,
		schema.Block,
		*inst.ProviderInstance,
		providerClient,
		inst.ConfigVal,
		shared.EphemeralResourceHooks{},
	)
	diags = diags.Append(openDiags)
	if openDiags.HasErrors() {
		return cty.NilVal, nil, diags
	}

	p.planCtx.closeStackMu.Lock()
	p.planCtx.closeStack = append(p.planCtx.closeStack, closeFunc)
	p.planCtx.closeStackMu.Unlock()

	resRef := egb.EphemeralResourceInstanceSubgraph(ctx, inst, newVal, p.oracle)
	return newVal, resRef, diags
}
