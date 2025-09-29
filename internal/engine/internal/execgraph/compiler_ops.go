// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"context"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func (c *compiler) compileOpOpenProvider(operands *compilerOperands) nodeExecuteRaw {
	getProviderAddr := nextOperand[addrs.Provider](operands)
	getConfigVal := nextOperand[cty.Value](operands)
	waitForDeps := operands.OperandWaiter()
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}

	providers := c.evalCtx.Providers

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		if !waitForDeps(ctx) {
			return nil, false, diags
		}
		providerAddr, ok, moreDiags := getProviderAddr(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}
		configVal, ok, moreDiags := getConfigVal(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}

		ret, moreDiags := providers.NewConfiguredProvider(ctx, providerAddr, configVal)
		diags = diags.Append(moreDiags)
		if moreDiags.HasErrors() {
			return nil, false, diags
		}
		return ret, true, diags
	}
}
