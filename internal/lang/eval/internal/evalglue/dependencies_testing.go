// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package evalglue

import (
	"context"
	"fmt"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ProvidersForTesting returns a [Providers] implementation that just returns
// information directly from the given map.
//
// This is intended for unit testing only.
func ProvidersForTesting(schemas map[addrs.Provider]*providers.GetProviderSchemaResponse) ProvidersSchema {
	return providersStatic{schemas}
}

// ProvisionersForTesting returns a [ProvisionersSchema] implementation that just
// returns information directly from the given map.
//
// This is intended for unit testing only.
func ProvisionersForTesting(schemas map[string]*configschema.Block) ProvisionersSchema {
	return provisionersStatic{schemas}
}

type providersStatic struct {
	schemas map[addrs.Provider]*providers.GetProviderSchemaResponse
}

// ProviderConfigSchema implements Providers.
func (p providersStatic) ProviderConfigSchema(ctx context.Context, provider addrs.Provider) (*providers.Schema, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	topSchema, ok := p.schemas[provider]
	if !ok {
		diags = diags.Append(fmt.Errorf("provider %q is not available to this test", provider))
		return nil, diags
	}
	return &topSchema.Provider, diags
}

// ValidateProviderConfig implements Providers by doing nothing at all, because
// in this implementation providers consist only of schema and have no behavior.
func (p providersStatic) ValidateProviderConfig(ctx context.Context, provider addrs.Provider, configVal cty.Value) tfdiags.Diagnostics {
	return nil
}

// ResourceTypeSchema implements Providers.
func (p providersStatic) ResourceTypeSchema(_ context.Context, provider addrs.Provider, mode addrs.ResourceMode, typeName string) (*providers.Schema, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	topSchema, ok := p.schemas[provider]
	if !ok {
		diags = diags.Append(fmt.Errorf("provider %q is not available to this test", provider))
		return nil, diags
	}
	var typesOfMode map[string]providers.Schema
	switch mode {
	case addrs.ManagedResourceMode:
		typesOfMode = topSchema.ResourceTypes
	case addrs.DataResourceMode:
		typesOfMode = topSchema.DataSources
	case addrs.EphemeralResourceMode:
		typesOfMode = topSchema.EphemeralResources
	default:
		typesOfMode = nil // no other modes supported
	}
	schema, ok := typesOfMode[typeName]
	if !ok {
		// The requirements for this interface say we should return nil to
		// represent that there is no such resource type, so that the caller
		// can provide its own error message for that.
		return nil, diags
	}
	return &schema, diags
}

// ValidateResourceConfig implements Providers by doing nothing at all, because
// in this implementation providers consist only of schema and have no behavior.
func (p providersStatic) ValidateResourceConfig(ctx context.Context, provider addrs.Provider, mode addrs.ResourceMode, typeName string, configVal cty.Value) tfdiags.Diagnostics {
	return nil
}

// NewConfiguredProvider implements Providers.
func (p providersStatic) NewConfiguredProvider(ctx context.Context, provider addrs.Provider, configVal cty.Value) (providers.Configured, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	diags = diags.Append(fmt.Errorf("only unconfigured providers are available to this test"))
	return nil, diags
}

type provisionersStatic struct {
	schemas map[string]*configschema.Block
}

// ProvisionerConfigSchema implements Provisioners.
func (p provisionersStatic) ProvisionerConfigSchema(_ context.Context, typeName string) (*configschema.Block, tfdiags.Diagnostics) {
	return p.schemas[typeName], nil
}
