// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package rpcproviders

import (
	"context"
	"io"

	"github.com/apparentlymart/opentofu-providers/tofuprovider"
	"github.com/apparentlymart/opentofu-providers/tofuprovider/providerops"
	"github.com/opentofu/opentofu/internal/providers"
)

type rpcProvider struct {
	client tofuprovider.Provider
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
	return rpcProvider{client}
}

// ConfigureProvider implements providers.Interface.
func (r rpcProvider) ConfigureProvider(ctx context.Context, req providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
	var resp providers.ConfigureProviderResponse
	clientResp, err := r.client.ConfigureProvider(ctx, &providerops.ConfigureProviderRequest{})
	resp.Diagnostics = appendDiags(resp.Diagnostics, clientResp, err)
	return resp
}

// ValidateProviderConfig implements providers.Interface.
func (r rpcProvider) ValidateProviderConfig(ctx context.Context, req providers.ValidateProviderConfigRequest) providers.ValidateProviderConfigResponse {
	panic("unimplemented")
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
