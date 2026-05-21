// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/common"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func newProviderInstances(planCtx *planContext, oracle *eval.PlanningOracle) *common.ProviderInstances {
	return common.NewProviderInstances(func(
		ctx context.Context,
		addr addrs.AbsProviderInstanceCorrect,
		closer func(context.Context) tfdiags.Diagnostics,
	) (providers.Configured, tfdiags.Diagnostics) {
		config := oracle.ProviderInstanceConfig(ctx, addr)
		if config == nil {
			// This suggests that the provider instance has an invalid
			// configuration. The main diagnostics for that get returned by
			// another channel but also return an error, so we just return
			// nil to prompt the caller to generate its own error saying that
			// whatever operation the provider was going to be used for cannot
			// be performed.
			//
			// FIXME: This currently doesn't handle the case where there's
			// an orphan or deposed resource instance object in the previous
			// run state referring to a provider instance whose configuration
			// was originally just implied to be empty by the existence of
			// some resource elsewhere in the configuration. Removing all
			// desired resource instances for such an implied provider when
			// there's still at least one object tracked in the state causes
			// us to return nil, here, whereas we ought to somehow attempt
			// to perform the implicit empty configuration behavior in that
			// case too.
			return nil, nil
		}
		configVal := config.ConfigVal

		// Since we've already evaluated the configuration now anyway, we'll
		// take this opportunity to record what it depends on for the benefit
		// of later analysis passes in the planning engine.
		// FIXME: We should probably have a more explicit API for this so that
		// we're not interacting directly with the unexported details of both
		// [planGlue] and [planCtx] here, but we'll wait to see how the rest
		// of the code in this package settles before deciding how to do it.
		planCtx.resourceInstObjs.PutProviderInstanceDependencies(addr, config.RequiredResourceInstances)

		// If _this_ call fails then unfortunately we'll end up duplicating
		// its diagnostics for every resource instance that depends on this
		// provider instance, which is not ideal but we don't currently have
		// any other return path for this problem. If this turns out to be
		// too annoying in practice then an alternative design would be to
		// have the [providerInstances] object track accumulated diagnostics
		// in one of its own fields and then make [planCtx.Close] pull those
		// all out at once after the planning work is complete. If we do that
		// then this should return "nil, nil" in the error case so that the
		// caller will treat it the same as a "configuration not valid enough"
		// problem.
		ret, diags := planCtx.providers.NewConfiguredProvider(ctx, addr.Config.Config.Provider, configVal)

		planCtx.closeStackMu.Lock()
		planCtx.closeStack = append(planCtx.closeStack, closer)
		planCtx.closeStackMu.Unlock()

		return ret, diags
	})
}
