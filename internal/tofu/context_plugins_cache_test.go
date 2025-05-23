// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"maps"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
)

// preloadAllProviderSchemasForUnitTest is a unit-testing-only helper method
// that simulates the effect of calling [contextPluginsCache.LoadProviderSchemas]
// with a configuration that makes use of all of the providers that are
// available in this [contextPlugins] object.
//
// Unlike the real LoadProviderSchemas method, this one does its work in the
// foreground and blocks until all schemas have been loaded.
//
// This is only for use in unit tests for components that typically expect
// that some other part of the system will have preloaded the schemas they
// need. It should not be used in context tests because the exported entrypoints
// of [Context] are supposed to arrange themselves for schemas to be loaded.
func (c *contextPluginsCache) preloadAllProviderSchemasForUnitTest(t *testing.T, factories map[addrs.Provider]providers.Factory) {
	c.providerSchemasMu.Lock()
	defer c.providerSchemasMu.Unlock()

	if c.providerSchemas == nil {
		c.providerSchemas = make(map[addrs.Provider]*providers.GetProviderSchemaResponse)
	}

	// Since we're reusing the same concurrent-loading helper the normal
	// load method uses, we'll still do the channel-related ceremony here.
	results := make(chan providerSchemaLoadResult)
	go c.loadSchemasForProviders(t.Context(), maps.Keys(factories), factories, results)
	for result := range results {
		c.providerSchemas[result.providerAddr] = result.response
	}
}
