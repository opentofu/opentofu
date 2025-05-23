// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"iter"
	"log"
	"maps"
	"slices"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// contextPluginsCache is the overall container for various information about
// provider and provisioner plugins that we expect should remain constant
// for the whole life of a [Context] object.
//
// Instances of this type are part of a [contextPlugins] object, and so the
// cached data is valid only for the plugins used by that object. This should
// be accessed only indirectly through the [contextPlugins] API.
type contextPluginsCache struct {
	// providerSchemas is a cache of previously-fetched provider schema
	// responses. We currently populate this all at once across all
	// providers so that subsequent code can assume that the schema for
	// any provider is always cheaply available in RAM.
	//
	// Access to this map must be coordinated through providerSchemasMu.
	providerSchemas   map[addrs.Provider]*providers.GetProviderSchemaResponse
	providerSchemasMu sync.RWMutex
}

// GetProviderSchemaResponse returns the full schema response from the given
// provider's "GetProviderSchema" function.
//
// The requested provider must have been included in an earlier call to
// [contextPluginsCache.LoadProviderSchemas] on the same cache object, or
// this will return a synthetic response containing an error diagnostic.
func (c *contextPluginsCache) GetProviderSchemaResponse(providerAddr addrs.Provider) *providers.GetProviderSchemaResponse {
	// This shared lock is mainly just to force us to block until any
	// background LoadProviderSchemas operation has completed. It's fine
	// for multiple readers to access the results of such a call concurrently.
	c.providerSchemasMu.RLock()
	ret, ok := c.providerSchemas[providerAddr]
	c.providerSchemasMu.RUnlock()
	if !ok {
		var diags tfdiags.Diagnostics
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Provider schema unavailable",
			fmt.Sprintf("OpenTofu needs to use the schema of provider %s, but it was not previously loaded from the provider. This is a bug in OpenTofu.", providerAddr),
		))
		ret = &providers.GetProviderSchemaResponse{
			Diagnostics: diags,
		}
	}
	return ret
}

// LoadProviderSchemas starts some background work to collect schema information
// for all of the providers used by the given configuration and state, both
// of which are optional and can be nil.
//
// This function returns immediately while schema loading work continues in the
// background. The other methods of this type related to provider schemas will
// block until all of the background work has completed, and so callers can
// assume that any future schema lookups will be handled only after the
// requests for all providers included in the given config and state have
// completed, but those later calls may take longer to return in order to
// satisfy that constraint.
//
// As an exception to the above rule, LoadProviderSchemas does _not_ return
// immediately if there is still a background task running from a previous
// call to the same function, and will instead block until that previous
// operation is complete to find out whether there's any new work left to do.
func (c *contextPluginsCache) LoadProviderSchemas(ctx context.Context, config *configs.Config, state *states.State, factories map[addrs.Provider]providers.Factory) {
	if config == nil && state == nil {
		// Nothing to do then. This is a silly situation but we'll handle it
		// here because it's easy to handle here.
		return
	}
	// We acquire the lock immediately here, before entering the goroutine
	// to make sure we always uphold the guarantee that any subsequent call
	// to fetch a provider schema will block until the background work is
	// complete. Otherwise it's possible for an early call to sneak in
	// before our goroutine begins running.
	log.Printf("[TRACE] contextPluginsCache.LoadProviderSchemas waiting for exclusive lock")
	c.providerSchemasMu.Lock()
	go func() {
		// We hold an exclusive lock on c.providerSchemas throughout our work
		// here so that subsequent lookups will block until we've had a change
		// to try to load everything.
		defer c.providerSchemasMu.Unlock()
		log.Printf("[TRACE] contextPluginsCache.LoadProviderSchemas begins")
		if c.providerSchemas == nil {
			c.providerSchemas = make(map[addrs.Provider]*providers.GetProviderSchemaResponse)
		}

		if config != nil {
			// We'll be loading schemas from multiple providers concurrently, but
			// we need to make sure our updates of c.providerSchemas happen only
			// sequentially and so we'll use a channel to gather the results.
			results := make(chan providerSchemaLoadResult)
			go c.loadSchemasForProviders(ctx, slices.Values(config.ProviderTypes()), factories, results)
			for result := range results {
				c.providerSchemas[result.providerAddr] = result.response
			}
		}

		if state != nil {
			// Whenever we have both a config and a state it's pretty rare for
			// there to be providers in the state that aren't in the config, so
			// we'll just deal with the state-only ones separately to avoid the
			// extra complexity of negotiating between two sets of concurrent
			// loads. Most of the time this will make no additional provider
			// schema requests at all, because we'll have already fetched
			// everything in the previous loop.
			results := make(chan providerSchemaLoadResult)
			go c.loadSchemasForProviders(ctx, maps.Keys(state.ProviderTypes()), factories, results)
			for result := range results {
				c.providerSchemas[result.providerAddr] = result.response
			}
		}

		log.Printf("[TRACE] contextPluginsCache.LoadProviderSchemas ends")

		// We could potentially choose to now throw away schema entries
		// for specific items that are not in config or state, but that
		// would mean we'd no longer be able to rely only on the direct
		// presence of keys in c.providerSchemas to detect if we can
		// reuse our existing cache entry, so we'll keep this simple for
		// now at the expense of keeping some unneeded data in RAM.
	}()
}

// providerSchemaLoadResult is an implementation detail of
// [contextPluginsCache.LoadProviderSchemas] and should not be used in any
// other way.
type providerSchemaLoadResult struct {
	providerAddr addrs.Provider
	response     *providers.GetProviderSchemaResponse
}

// loadProviderSchemas fetches the schema for any provider in providerAddrs
// that isn't already in c.providerSchemas and writes the result to the
// channel given in into, and then closes that channel when the work is
// all done.
//
// This may be called only from [contextPluginsCache.LoadProviderSchemas]
// while holding an exclusive lock on the providerSchemas map, and the
// providerSchemas must not be concurrently modified until at least one
// item has been written to the channel or the channel has been closed.
func (c *contextPluginsCache) loadSchemasForProviders(ctx context.Context, providerAddrs iter.Seq[addrs.Provider], factories map[addrs.Provider]providers.Factory, into chan<- providerSchemaLoadResult) {
	// To allow the caller to modify c.providerSchemas each time we
	// emit a result we'll first collect a copy of the set of providers
	// we already alreadyHave cached results for, which we'll then use instead
	// of directly accessing the map in our main loop below.
	alreadyHave := make(map[addrs.Provider]struct{}, len(c.providerSchemas))
	for providerAddr := range c.providerSchemas {
		alreadyHave[providerAddr] = struct{}{}
	}
	// After this point we must not access c.providerSchemas anymore.

	log.Printf("[TRACE] contextPluginsCache.loadSchemasForProviders starting loop")
	var wg sync.WaitGroup
	for providerAddr := range providerAddrs {
		if _, exists := alreadyHave[providerAddr]; exists {
			continue // we already have this one
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := loadProviderSchema(ctx, providerAddr, factories)
			into <- providerSchemaLoadResult{
				providerAddr: providerAddr,
				response:     resp,
			}
		}()
	}
	log.Printf("[TRACE] contextPluginsCache.loadSchemasForProviders waiting for goroutines to complete")
	wg.Wait()
	log.Printf("[TRACE] contextPluginsCache.loadSchemasForProviders goroutines are complete, so closing channel")
	close(into) // all done!
	log.Printf("[TRACE] contextPluginsCache.loadSchemasForProviders all done")
}

// loadProviderSchema does the main work of retrieving the schema for a
// particular provider, called from one of the loops in
// [contextPluginsCache.LoadProviderSchemas].
//
// This function intentionally has no direct access to the [contextPluginsCache]
// object that it was called for so that it can safely run concurrently with
// other calls fetching other providers without any data races. The caller
// is responsible for whatever synchronization is needed to safely store the
// result once this function returns.
//
// This function always returns a response object, but in some cases that object
// is synthetically-constructed within the function as a way to report
// diagnostics about problems that prevented interacting with the provider at
// all.
func loadProviderSchema(ctx context.Context, providerAddr addrs.Provider, factories map[addrs.Provider]providers.Factory) *providers.GetProviderSchemaResponse {
	log.Printf("[TRACE] contextPluginsCache.loadProviderSchema for %s", providerAddr)

	factory, ok := factories[providerAddr]
	if !ok {
		// Should not get here: this means that we are trying to load a schema
		// from a provider we don't have, which suggests a bug in whatever
		// called tofu.NewContext and one of the methods on the result.
		// We'll still return a reasonable diagnostic for it though, for
		// robustness.
		log.Printf("[TRACE] contextPluginsCache.loadProviderSchema no factory for %s", providerAddr)
		var diags tfdiags.Diagnostics
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Schema request for unavailable provider",
			fmt.Sprintf("OpenTofu needs to load a schema for provider %s, but that provider is not available in this execution context. This is a bug in OpenTofu.", providerAddr),
		))
		return &providers.GetProviderSchemaResponse{
			Diagnostics: diags,
		}
	}

	provider, err := factory()
	if err != nil {
		log.Printf("[TRACE] contextPluginsCache.loadProviderSchema failed to start %s", providerAddr)
		var diags tfdiags.Diagnostics
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to start provider",
			fmt.Sprintf("Unable to start provider %q to fetch its schema: %s.", providerAddr, tfdiags.FormatError(err)),
		))
		return &providers.GetProviderSchemaResponse{
			Diagnostics: diags,
		}
	}
	defer provider.Close(ctx)

	log.Printf("[TRACE] contextPluginsCache.loadProviderSchema GetProviderSchema starting for %s", providerAddr)
	resp := provider.GetProviderSchema(ctx)
	log.Printf("[TRACE] contextPluginsCache.loadProviderSchema GetProviderSchema completed for %s", providerAddr)
	// We'll also add any schema validation errors into the response, so that
	// callers have only one place to check for all possible errors. This
	// does mean that the errors about the response are embedded in that same
	// response, which is a little weird but also true for any errors that
	// could be returned by the provider itself, and so callers need to be
	// prepared for that situation anyway.
	resp.Diagnostics = append(resp.Diagnostics, validateProviderSchemaResponse(providerAddr, &resp)...)
	return &resp
}
