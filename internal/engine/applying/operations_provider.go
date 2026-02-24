// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"fmt"
	"log"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ProviderInstanceConfig implements [exec.Operations].
func (ops *execOperations) ProviderInstanceConfig(
	ctx context.Context,
	instAddr addrs.AbsProviderInstanceCorrect,
) (*exec.ProviderInstanceConfig, tfdiags.Diagnostics) {
	log.Printf("[TRACE] apply phase: ProviderInstanceConfig %s", instAddr)
	configVal, diags := ops.configOracle.ProviderInstanceConfig(ctx, instAddr)
	if configVal == cty.NilVal {
		configVal = cty.DynamicVal
	}
	if diags.HasErrors() {
		configVal = exprs.AsEvalError(configVal)
	}
	return &exec.ProviderInstanceConfig{
		InstanceAddr: instAddr,
		ConfigVal:    configVal,
	}, diags
}

// ProviderInstanceOpen implements [exec.Operations].
func (ops *execOperations) ProviderInstanceOpen(
	ctx context.Context,
	config *exec.ProviderInstanceConfig,
) (*exec.ProviderClient, tfdiags.Diagnostics) {
	log.Printf("[TRACE] apply phase: ProviderInstanceOpen %s", config.InstanceAddr)
	provider := config.InstanceAddr.Config.Config.Provider
	realClient, diags := ops.plugins.NewConfiguredProvider(ctx, provider, config.ConfigVal)
	if realClient == nil {
		return nil, diags
	}
	return &exec.ProviderClient{
		InstanceAddr: config.InstanceAddr,
		Ops:          realClient,
	}, diags
}

// ProviderInstanceClose implements [exec.Operations].
func (ops *execOperations) ProviderInstanceClose(
	ctx context.Context,
	client *exec.ProviderClient,
) tfdiags.Diagnostics {
	log.Printf("[TRACE] apply phase: ProviderInstanceClose %s", client.InstanceAddr)
	var diags tfdiags.Diagnostics
	err := client.Ops.Close(ctx)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error while closing provider",
			fmt.Sprintf("Failed to close the provider plugin for %s: %s.", client.InstanceAddr, tfdiags.FormatError(err)),
		))
	}
	return diags
}
