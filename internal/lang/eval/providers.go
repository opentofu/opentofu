// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"
	"fmt"
	"log"
	"slices"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/shared"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

type providerConfigSupplier func(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) (cty.Value, tfdiags.Diagnostics)

func newManagedProviders(unconfigured Providers, config providerConfigSupplier) *managedProviders {
	return &managedProviders{
		Providers: unconfigured,
		config:    config,
		active:    addrs.MakeMap[addrs.AbsProviderInstanceCorrect, *grapheval.Once[providers.Configured]](),
	}
}

// managedProviders represents all of the active providers
// within a given operation.
type managedProviders struct {
	Providers

	config providerConfigSupplier
	// active contains a grapheval.Once for each provider instance that
	// has previously been requested, which resolve once the provider instance
	// is configured and ready to use.
	//
	// callers must hold activeMu while accessing this map but should release it
	// before waiting on an object retrieved from it.
	active   addrs.Map[addrs.AbsProviderInstanceCorrect, *grapheval.Once[providers.Configured]]
	activeMu sync.Mutex

	// Stack of ephemeral and provider close functions
	// Given the current state of the planning engine, we wait until
	// the end of the run to close all of the "opened" items.  We
	// also need to close them in a specific order to prevent dependency
	// conflicts. We posit that for plan, closing in the reverse order of opens
	// will ensure that this order is correctly preserved.
	closeStackMu sync.Mutex
	closeStack   []func(context.Context) tfdiags.Diagnostics
}

func (p *managedProviders) ProviderInstance(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) (providers.Interface, tfdiags.Diagnostics) {
	p.activeMu.Lock()
	if !p.active.Has(addr) {
		p.active.Put(addr, &grapheval.Once[providers.Configured]{})
	}
	once := p.active.Get(addr)
	p.activeMu.Unlock()

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

		configVal, diags := p.config(ctx, addr)
		if diags.HasErrors() || configVal == cty.NilVal {
			return nil, diags
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
		ret, moreDiags := p.NewConfiguredProvider(ctx, addr.Config.Config.Provider, configVal)
		diags = diags.Append(moreDiags)
		if diags.HasErrors() {
			return nil, diags
		}

		p.closeStackMu.Lock()
		p.closeStack = append(p.closeStack, closer)
		p.closeStackMu.Unlock()

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

func (p *managedProviders) OpenEphemeralResourceInstance(ctx context.Context, addr addrs.AbsResourceInstance, cfgVal cty.Value, provider addrs.Provider, providerInstance *addrs.AbsProviderInstanceCorrect) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	schema, _ := p.ResourceTypeSchema(ctx, provider, addr.Resource.Resource.Mode, addr.Resource.Resource.Type)
	if schema == nil || schema.Block == nil {
		// Should be caught during validation, so we don't bother with a pretty error here
		diags = diags.Append(fmt.Errorf("provider %q does not support ephemeral resource %q", providerInstance, addr.Resource.Resource.Type))
		return cty.NilVal, diags
	}

	objTy := schema.Block.ImpliedType()
	priorVal := cty.NullVal(objTy)

	if providerInstance == nil {
		// If we don't even know which provider instance we're supposed to be
		// talking to then we'll just return a placeholder value, because
		// we don't have any way to generate a speculative plan.
		return priorVal, diags
	}

	// TODO the old engine also looks at depends_on
	if !cfgVal.IsWhollyKnown() {
		return priorVal, diags
	}

	providerClient, moreDiags := p.ProviderInstance(ctx, *providerInstance)
	if providerClient == nil {
		moreDiags = moreDiags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Provider instance not available",
			fmt.Sprintf("Cannot plan %s because its associated provider instance %s cannot initialize.", addr, *providerInstance),
			nil,
		))
	}
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return priorVal, diags
	}

	newVal, closeFunc, openDiags := shared.OpenEphemeralResourceInstance(
		ctx,
		addr,
		schema.Block,
		*providerInstance,
		providerClient,
		cfgVal,
		shared.EphemeralResourceHooks{},
	)
	diags = diags.Append(openDiags)
	if openDiags.HasErrors() {
		return priorVal, diags
	}

	p.closeStackMu.Lock()
	p.closeStack = append(p.closeStack, closeFunc)
	p.closeStackMu.Unlock()

	return newVal, diags
}

// ConfiguredFunction constructs a cty function given a provider and a function address.
func (p *managedProviders) ConfiguredFunction(ctx context.Context, providerInstance addrs.AbsProviderInstanceCorrect, pf addrs.ProviderFunction, rng hcl.Range) (function.Function, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	providerClient, moreDiags := p.ProviderInstance(ctx, providerInstance)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return function.Function{}, diags
	}

	return providers.BuildProviderFunction(ctx, providerClient, pf, false, rng)
}

func (p *managedProviders) Close(ctx context.Context) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	p.closeStackMu.Lock()
	slices.Reverse(p.closeStack)
	for _, closer := range p.closeStack {
		diags = diags.Append(closer(ctx))
	}
	p.closeStackMu.Unlock()

	// TODO should we close the unmanaged providers here?
	// diags = diags.Append(p.Providers.Close(ctx))

	return diags
}
