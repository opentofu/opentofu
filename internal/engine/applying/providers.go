// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/common"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func newProviderInstances(execOperations *execOperations) *common.ProviderInstances {
	return common.NewProviderInstances(func(
		ctx context.Context,
		addr addrs.AbsProviderInstanceCorrect,
		closer func(context.Context) tfdiags.Diagnostics,
	) (providers.Configured, tfdiags.Diagnostics) {
		oracle := execOperations.configOracle

		configVal, diags := oracle.ProviderInstanceConfig(ctx, addr)
		if diags.HasErrors() {
			return nil, diags
		}

		// If _this_ call fails then unfortunately we'll end up duplicating
		// its diagnostics for every resource instance that depends on this
		// provider instance, which is not ideal but we don't currently have
		// any other return path for this problem. If this turns out to be
		// too annoying in practice then an alternative design would be to
		// have the [providerInstances] object track accumulated diagnostics
		// in one of its own fields and then make [execOperations.Close] pull those
		// all out at once after the applyning work is complete. If we do that
		// then this should return "nil, nil" in the error case so that the
		// caller will treat it the same as a "configuration not valid enough"
		// problem.
		ret, diags := execOperations.plugins.NewConfiguredProvider(ctx, addr.Config.Config.Provider, configVal)

		execOperations.closeStackMu.Lock()
		execOperations.closeStack = append(execOperations.closeStack, closer)
		execOperations.closeStackMu.Unlock()

		return ret, diags
	})
}
