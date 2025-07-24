// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package rpcproviders

import (
	"context"
	"io"
	"log"

	"github.com/apparentlymart/opentofu-providers/tofuprovider"
	"github.com/apparentlymart/opentofu-providers/tofuprovider/providerops"
	"github.com/apparentlymart/opentofu-providers/tofuprovider/providerschema"
	"github.com/opentofu/opentofu/internal/providers"
)

type rpcProvider struct {
	client tofuprovider.Provider

	// schema is a caching helper for schema information.
	//
	// This is kept as a separate object so that in future we might choose
	// to share it across multiple instances of the same provider on the
	// assumption that the schema should be static, but for now we treat
	// each provider separately.
	schema *schemaCache
}

// NewProvider wraps the given provider client in a [providers.Interface]
// implementation, so that most methods of the result will delegate to
// corresponding methods of that client.
//
// If the given client also implements [io.Closer] then the Close method
// of the result will call it. Otherwise Close is a no-op.
//
// It's the caller's responsibility to actually start the provider and
// obtain the client.
func NewProvider(client tofuprovider.Provider) providers.Interface {
	return rpcProvider{
		client: client,

		// For now we always make a new schema cache, but the intent of this
		// design is that in future we could offer an alternative to this
		// NewProvider function which takes an additional argument of
		// an existing providers.Interface that we should share the schema
		// cache with if possible, and then callers can arrange to use their
		// initial unconfigured provider instance to handle the schema
		// fetching and caching for all subsequent _configured_ instances
		// of the provider, so that the shared cache is always populated
		// from the same instance of the provider.
		//
		// However, if you're looking at this code thinking about introducing
		// something like that then note that we're currently relying on these
		// schema caches _not_ being shared because OpenTofu allows a provider
		// to return different functions after it has been configured, and
		// so each instance can potentially have different functions. We'll
		// need to split this up a little differently if we actually do want
		// to share the schema cache between instances later.
		schema: &schemaCache{
			client: client,
		},
	}
}

// ConfigureProvider implements providers.Interface.
func (r rpcProvider) ConfigureProvider(ctx context.Context, req providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
	log.Printf("[TRACE] rpcProvider.ConfigureProvider")
	var resp providers.ConfigureProviderResponse

	schema, diags := r.schema.GetProviderConfig(ctx)
	resp.Diagnostics = resp.Diagnostics.Append(diags)
	if resp.Diagnostics.HasErrors() {
		return resp
	}

	clientResp, err := r.client.ConfigureProvider(ctx, &providerops.ConfigureProviderRequest{
		Config:             providerschema.NewDynamicValue(req.Config, schema.Block.ImpliedType()),
		ClientCapabilities: clientCapabilities,
	})
	resp.Diagnostics = appendDiags(resp.Diagnostics, clientResp, err)

	// OpenTofu allows providers to return different functions based on how
	// they are configured, so we need to invalidate the functions cache
	// when we are configured. This is technically a little racy because
	// a caller could potentially try to call a function concurrently
	// with the provider being configured and so get an unpredictable result,
	// but in practice OpenTofu's language runtime ensures that functions are
	// never called concurrently with configuring the provider they come from.
	//
	// If we switch to a model where we share schema cache objects between
	// multiple instances of rpcProvider then we'll need to adopt a different
	// strategy where each provider instance has its own independent function
	// signature cache despite sharing everything else.
	r.schema.InvalidateCachedFunctions()

	return resp
}

// ValidateProviderConfig implements providers.Interface.
func (r rpcProvider) ValidateProviderConfig(ctx context.Context, req providers.ValidateProviderConfigRequest) providers.ValidateProviderConfigResponse {
	log.Printf("[TRACE] rpcProvider.ValidateProviderConfig")
	var resp providers.ValidateProviderConfigResponse

	schema, diags := r.schema.GetProviderConfig(ctx)
	resp.Diagnostics = resp.Diagnostics.Append(diags)
	if resp.Diagnostics.HasErrors() {
		return resp
	}

	clientResp, err := r.client.ConfigureProvider(ctx, &providerops.ConfigureProviderRequest{
		Config:             providerschema.NewDynamicValue(req.Config, schema.Block.ImpliedType()),
		ClientCapabilities: clientCapabilities,
	})
	resp.Diagnostics = appendDiags(resp.Diagnostics, clientResp, err)
	return resp
}

// Stop implements providers.Interface.
func (r rpcProvider) Stop(ctx context.Context) error {
	return r.client.GracefulStop(ctx)
}

// Close delegates to the same method of the inner client if the internal
// client implements [io.Closer]. Otherwise it's a no-op.
func (r rpcProvider) Close(_ context.Context) error {
	if closer, ok := r.client.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
