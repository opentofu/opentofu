// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

func (p *planGlue) ValidateProviderConfig(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, configVal cty.Value, riDeps addrs.Set[addrs.AbsResourceInstance]) (func(ctx context.Context) (providers.Configured, tfdiags.Diagnostics), tfdiags.Diagnostics) {
	egb := p.planCtx.execGraphBuilder

	diags := p.planCtx.providers.ValidateProviderConfig(ctx, addr.Config.Config.Provider, configVal)

	return func(ctx context.Context) (providers.Configured, tfdiags.Diagnostics) {
		egb.ProviderInstanceSubgraph(addr, riDeps)

		if configVal == cty.NilVal {
			// This suggests that the provider instance has an invalid
			// configuration. The main diagnostics for that get returned by
			// another channel but also return an error, so we just return
			// nil to prompt the caller to generate its own error saying that
			// whatever operation the provider was going to be used for cannot
			// be performed.
			return nil, nil
		}

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
		ret, diags := p.planCtx.providers.NewConfiguredProvider(ctx, addr.Config.Config.Provider, configVal)

		closeCh := make(chan struct{})

		p.planCtx.closeStackMu.Lock()
		p.planCtx.closeStack = append(p.planCtx.closeStack, func(ctx context.Context) tfdiags.Diagnostics {
			println("CLOSING PROVIDER " + addr.String())
			closeCh <- struct{}{}
			return tfdiags.Diagnostics{}.Append(ret.Close(ctx))
		})
		p.planCtx.closeStackMu.Unlock()

		// This background goroutine deals with closing the provider once it's
		// no longer needed, and with asking it to gracefully stop if our
		// given context is cancelled.
		go func() {
			cancelCtx := ctx
			withoutCancelCtx := context.WithoutCancel(ctx)
			for {
				select {
				case <-closeCh:
					// Close() has been called from within the closers
					// No further actions are nessesary
					return
				case <-cancelCtx.Done():
					// If the context we were given is cancelled then we'll
					// ask the provider to perform a graceful stop so that
					// active requests to the provider are more likely to
					// terminate soon.
					if ret != nil {
						_ = ret.Stop(withoutCancelCtx)
					}
					return
				}
			}
		}()

		return ret, diags
	}, diags
}
