// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package memory

import (
	"context"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
)

// Provider is an implementation of providers.Interface
type Provider struct {
}

// NewProvider returns a new instance of the "memory" provider.
func NewProvider() providers.Interface {
	return &Provider{}
}

// Close implements providers.Interface.
func (p *Provider) Close(context.Context) error {
	return nil
}

// ConfigureProvider implements providers.Interface.
func (p *Provider) ConfigureProvider(context.Context, providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
	return providers.ConfigureProviderResponse{}
}

// GetFunctions implements providers.Interface.
func (p *Provider) GetFunctions(context.Context) providers.GetFunctionsResponse {
	return providers.GetFunctionsResponse{}
}

// GetProviderSchema implements providers.Interface.
func (p *Provider) GetProviderSchema(context.Context) providers.GetProviderSchemaResponse {
	return providers.GetProviderSchemaResponse{
		Provider: providers.Schema{
			Block: &configschema.Block{
				// This provider expects no configuration arguments.
			},
		},
		ResourceTypes: map[string]providers.Schema{
			"memory": resourceTypeSchema(),
		},
	}
}

// Stop implements providers.Interface.
func (p *Provider) Stop(context.Context) error {
	return nil
}

// ValidateProviderConfig implements providers.Interface.
func (p *Provider) ValidateProviderConfig(context.Context, providers.ValidateProviderConfigRequest) providers.ValidateProviderConfigResponse {
	return providers.ValidateProviderConfigResponse{}
}
