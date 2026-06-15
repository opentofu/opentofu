// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"log"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type ConfiguredProviderInstanceFunc func(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, closer func(context.Context) tfdiags.Diagnostics) (providers.Configured, tfdiags.Diagnostics)

// ProviderInstances is our central manager of active configured provider
// instances, responsible for executing new providers on request and for
// keeping them running until all of their work is done.
type ProviderInstances struct {
	// active contains a grapheval.Once for each provider instance that
	// has previously been requested, which resolve once the provider instance
	// is configured and ready to use.
	//
	// callers must hold activeMu while accessing this map but should release it
	// before waiting on an object retrieved from it.
	active   addrs.Map[addrs.AbsProviderInstanceCorrect, *grapheval.Once[providers.Configured]]
	activeMu sync.Mutex

	newConfiguredProvider ConfiguredProviderInstanceFunc
}

func NewProviderInstances(newConfiguredProvider ConfiguredProviderInstanceFunc) *ProviderInstances {
	return &ProviderInstances{
		active:                addrs.MakeMap[addrs.AbsProviderInstanceCorrect, *grapheval.Once[providers.Configured]](),
		newConfiguredProvider: newConfiguredProvider,
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
func (pi *ProviderInstances) ProviderClient(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) (providers.Configured, tfdiags.Diagnostics) {
	// We hold this central lock only while we make sure there's an entry
	// in the "active" map for this provider instance. We then use the
	// more granular grapheval.Once inside to wait for the provider client
	// to be available, so that requests for already-active provider instances
	// will not block on the startup of other provider instances.
	pi.activeMu.Lock()
	if !pi.active.Has(addr) {
		pi.active.Put(addr, &grapheval.Once[providers.Configured]{})
	}
	once := pi.active.Get(addr)
	pi.activeMu.Unlock()

	return once.Do(ctx, func(ctx context.Context) (ret providers.Configured, diags tfdiags.Diagnostics) {
		log.Printf("[INFO] Opening Provider %s", addr)

		closeCh := make(chan struct{})
		closer := func(ctx context.Context) tfdiags.Diagnostics {
			log.Printf("[INFO] Closing Provider %s", addr)
			closeCh <- struct{}{}
			if ret != nil {
				return tfdiags.Diagnostics{}.Append(ret.Close(ctx))
			}
			return nil
		}

		ret, diags = pi.newConfiguredProvider(ctx, addr, closer)

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
					log.Printf("[INFO] Stopping Provider %s", addr)
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
	})
}
