// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// providerInstances is our central manager of active configured provider
// instances, responsible for executing new providers on request and for
// keeping them running until all of their work is done.
type providerInstances struct {

	// active contains a grapheval.Once for each provider instance that
	// has previously been requested, which resolve once the provider instance
	// is configured and ready to use.
	//
	// callers must hold activeMu while accessing this map but should release it
	// before waiting on an object retrieved from it.
	active   addrs.Map[addrs.AbsProviderInstanceCorrect, *grapheval.Once[providers.Configured]]
	activeMu sync.Mutex

	// completion is a [completionTracker] that's shared with the [planCtx]
	// we belong to so that we can detect when all of the work of each particular
	// provider instance has completed.
	completion *completionTracker
}

func newProviderInstances(completion *completionTracker) *providerInstances {
	return &providerInstances{
		active:     addrs.MakeMap[addrs.AbsProviderInstanceCorrect, *grapheval.Once[providers.Configured]](),
		completion: completion,
	}
}

// ProviderClient returns a client for the requested provider instance, using
// information from the given planGlue to configure the provider if no caller has
// previously requested a client for this instance.
//
// (It's better to enter through [planGlue.providerClient], which is a wrapper
// that passes its receiver into the final argument here.)
//
// Returns nil if the configuration for the requested provider instance is too
// invalid to actually configure it. The diagnostics for such a problem would
// be reported by our main [ConfigInstance.DrivePlanning] call but the caller
// of this function will probably want to return a more specialized error saying
// that the corresponding resource cannot be planned because its associated
// provider has an invalid configuration.
func (pi *providerInstances) ProviderClient(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, planGlue *planGlue) (providers.Configured, tfdiags.Diagnostics) {
	// We hold this central lock only while we make sure there's an entry
	// in the "active" map for this provider instance. We then use the
	// more granular grapheval.Once inside to wait for the provider client
	// to be available, so that requests for already-active provider instances
	// will not block on the startup of other provider instances.
	pi.activeMu.Lock()
	if !pi.active.Has(addr) {
		pi.active.Put(addr, &grapheval.Once[providers.Configured]{})
	}
	pi.activeMu.Unlock()

	oracle := planGlue.oracle
	once := pi.active.Get(addr)
	return once.Do(ctx, func(ctx context.Context) (providers.Configured, tfdiags.Diagnostics) {
		configVal := oracle.ProviderInstanceConfig(ctx, addr)
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
		ret, diags := planGlue.planCtx.providers.NewConfiguredProvider(ctx, addr.Config.Config.Provider, configVal)

		// This background goroutine deals with closing the provider once it's
		// no longer needed, and with asking it to gracefully stop if our
		// given context is cancelled.
		waitCh := pi.completion.NewWaiterFor(planGlue.providerInstanceCompletionEvents(ctx, addr))
		go func() {
			// Once this goroutine is complete the provider instance should be
			// treated as closed.
			defer planGlue.planCtx.reportProviderInstanceClosed(addr)

			cancelCtx := ctx
			withoutCancelCtx := context.WithoutCancel(ctx)
			for {
				select {
				case <-waitCh:
					// Everything that we were expecting to use the provider
					// instance has now completed, so we can close it.
					//
					// (An error from this goes nowhere. If we want to track
					// this then maybe we _should_ switch to having a central
					// diags repository inside the providerInstances object,
					// as discussed above for failing NewConfiguredProvider,
					// and then we could write this failure into there.)
					if ret != nil {
						_ = ret.Close(withoutCancelCtx)
					}
					return
				case <-cancelCtx.Done():
					// If the context we were given is cancelled then we'll
					// ask the provider to perform a graceful stop so that
					// active requests to the provider are more likely to
					// terminate soon.
					if ret != nil {
						_ = ret.Stop(withoutCancelCtx)
					}
					// We'll now replace cancelCtx with the one guaranteed
					// to never be cancelled so that we'll block until waitCh
					// is closed.
					cancelCtx = withoutCancelCtx
				}
			}
		}()

		return ret, diags
	})
}
