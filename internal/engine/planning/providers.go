// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
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
	active   addrs.Map[addrs.AbsProviderInstanceCorrect, *grapheval.Once[providerInstance]]
	activeMu sync.Mutex

	closers   addrs.Map[addrs.AbsProviderInstanceCorrect, func(context.Context) error]
	closersMu sync.Mutex
}

func newProviderInstances() *providerInstances {
	return &providerInstances{
		active:  addrs.MakeMap[addrs.AbsProviderInstanceCorrect, *grapheval.Once[providerInstance]](),
		closers: addrs.MakeMap[addrs.AbsProviderInstanceCorrect, func(context.Context) error](),
	}
}

type providerInstance struct {
	instance             providers.Configured
	ref                  execgraph.ResultRef[*exec.ProviderClient]
	registerCloseBlocker execgraph.RegisterCloseBlockerFunc
}

func (p *providerInstances) callClose(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) error {
	// This is called after any modifications to the map is active
	return p.closers.Get(addr)(ctx)
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
func (p *providerInstances) ProviderClient(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, planGlue *planGlue) (providers.Configured, execgraph.ResultRef[*exec.ProviderClient], execgraph.RegisterCloseBlockerFunc, tfdiags.Diagnostics) {
	// We hold this central lock only while we make sure there's an entry
	// in the "active" map for this provider instance. We then use the
	// more granular grapheval.Once inside to wait for the provider client
	// to be available, so that requests for already-active provider instances
	// will not block on the startup of other provider instances.
	p.activeMu.Lock()
	if !p.active.Has(addr) {
		p.active.Put(addr, &grapheval.Once[providerInstance]{})
	}
	p.activeMu.Unlock()

	oracle := planGlue.oracle
	egb := planGlue.planCtx.execgraphBuilder
	once := p.active.Get(addr)
	val, diags := once.Do(ctx, func(ctx context.Context) (providerInstance, tfdiags.Diagnostics) {
		var result providerInstance

		resourceDependencies := oracle.ProviderInstanceResourceDependencies(ctx, addr)

		var dependencyResults []execgraph.AnyResultRef
		for depInst := range resourceDependencies {
			depInstResult := egb.ResourceInstanceFinalStateResult(depInst.Addr)
			dependencyResults = append(dependencyResults, depInstResult)
		}
		dependencyWaiter := egb.Waiter(dependencyResults...)

		//result.ref, result.registerCloseBlocker = egb.ProviderInstance(addr, dependencyWaiter)

		addrResult := egb.ConstantProviderInstAddr(addr)
		configResult := egb.ProviderInstanceConfig(addrResult, dependencyWaiter)
		openResult := egb.ProviderInstanceOpen(configResult)
		closeWait, registerCloseBlocker := egb.MakeCloseBlocker()
		closeResult := egb.ProviderInstanceClose(openResult, closeWait)

		result.ref = openResult
		result.registerCloseBlocker = registerCloseBlocker

		configVal := oracle.ProviderInstanceConfig(ctx, addr)
		if configVal == cty.NilVal {
			// This suggests that the provider instance has an invalid
			// configuration. The main diagnostics for that get returned by
			// another channel but also return an error, so we just return
			// nil to prompt the caller to generate its own error saying that
			// whatever operation the provider was going to be used for cannot
			// be performed.
			return result, nil
		}

		for depInst := range resourceDependencies {
			if depInst.Addr.Resource.Resource.Mode == addrs.EphemeralResourceMode {
				// Our open was dependent on an ephemeral's open,
				// therefore the ephemeral's close should depend on our close
				//
				// The dependency should already have been populated via planDesiredEphemeralResourceInstance
				// TODO it's unclear if we can do this before or after the oracle call above
				planGlue.planCtx.ephemeralInstances.addCloseDependsOn(depInst.Addr, closeResult)
			}
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
		result.instance = ret

		// Mark this as "closed".  The planGraphCloseOperations will take care of it properly later
		planGlue.planCtx.reportProviderInstanceClosed(addr)

		closeCh := make(chan struct{})

		// Register closer for use in planGraphCloseOperations
		p.closersMu.Lock()
		p.closers.Put(addr, func(ctx context.Context) error {
			println("CLOSING PROVIDER " + addr.String())
			closeCh <- struct{}{}
			return ret.Close(ctx)
		})
		p.closersMu.Unlock()

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

		return result, diags
	})

	return val.instance, val.ref, val.registerCloseBlocker, diags
}
