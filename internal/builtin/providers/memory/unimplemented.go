// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package memory

import (
	"context"

	"github.com/opentofu/opentofu/internal/providers"
)

// The following are all of the methods of providers.Interface that this
// provider does not implement because they relate to features that do not
// appear in the provider's schema at all.

// CallFunction implements providers.Interface.
func (p *Provider) CallFunction(context.Context, providers.CallFunctionRequest) providers.CallFunctionResponse {
	panic("unimplemented")
}

// CloseEphemeralResource implements providers.Interface.
func (p *Provider) CloseEphemeralResource(context.Context, providers.CloseEphemeralResourceRequest) providers.CloseEphemeralResourceResponse {
	panic("unimplemented")
}

// OpenEphemeralResource implements providers.Interface.
func (p *Provider) OpenEphemeralResource(context.Context, providers.OpenEphemeralResourceRequest) providers.OpenEphemeralResourceResponse {
	panic("unimplemented")
}

// ReadDataSource implements providers.Interface.
func (p *Provider) ReadDataSource(context.Context, providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
	panic("unimplemented")
}

// RenewEphemeralResource implements providers.Interface.
func (p *Provider) RenewEphemeralResource(context.Context, providers.RenewEphemeralResourceRequest) (resp providers.RenewEphemeralResourceResponse) {
	panic("unimplemented")
}

// ValidateDataResourceConfig implements providers.Interface.
func (p *Provider) ValidateDataResourceConfig(context.Context, providers.ValidateDataResourceConfigRequest) providers.ValidateDataResourceConfigResponse {
	panic("unimplemented")
}

// ValidateEphemeralConfig implements providers.Interface.
func (p *Provider) ValidateEphemeralConfig(context.Context, providers.ValidateEphemeralConfigRequest) providers.ValidateEphemeralConfigResponse {
	panic("unimplemented")
}
