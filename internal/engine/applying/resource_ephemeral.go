// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/shared"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

func (e *evalGlue) OpenEphemeralResourceInstance(ctx context.Context, addr addrs.AbsResourceInstance, cfgVal cty.Value, providerInstance addrs.AbsProviderInstanceCorrect) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	schema, _ := e.plugins.ResourceTypeSchema(ctx, providerInstance.Config.Config.Provider, addr.Resource.Resource.Mode, addr.Resource.Resource.Type)
	if schema == nil || schema.Block == nil {
		// Should be caught during validation, so we don't bother with a pretty error here
		diags = diags.Append(fmt.Errorf("provider %q does not support ephemeral resource %q", providerInstance, addr.Resource.Resource.Type))
		return cty.NilVal, diags
	}

	providerClient, moreDiags := e.ops.providerInstances.ProviderClient(ctx, providerInstance)
	if providerClient == nil {
		moreDiags = moreDiags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Provider instance not available",
			fmt.Sprintf("Cannot plan %s because its associated provider instance %s cannot initialize.", addr, providerInstance),
			nil,
		))
	}
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return cty.NilVal, diags
	}

	newVal, closeFunc, openDiags := shared.OpenEphemeralResourceInstance(
		ctx,
		addr,
		schema.Block,
		providerInstance,
		providerClient,
		cfgVal,
		shared.EphemeralResourceHooks{},
	)
	diags = diags.Append(openDiags)
	if openDiags.HasErrors() {
		return cty.NilVal, diags
	}

	e.ops.closeStackMu.Lock()
	e.ops.closeStack = append(e.ops.closeStack, closeFunc)
	e.ops.closeStackMu.Unlock()

	return newVal, diags
}
