// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package evalglue

import (
	"context"
	"fmt"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ModulesForTesting returns an [ExternalModules] implementation that just
// returns module objects directly from the provided map, without any additional
// logic.
//
// This is intended for unit testing only, and only supports local module
// source addresses because it has no means to resolve remote sources or
// selected versions for registry-based modules.
//
// [configs.ModulesFromStringsForTesting] is a convenient way to build a
// suitable map to pass to this function when the required configuration is
// relatively small.
func ModulesForTesting(modules map[addrs.ModuleSourceLocal]*configs.Module) ExternalModules {
	return externalModulesStatic{modules}
}

// ProvidersForTesting returns a [Providers] implementation that just returns
// information directly from the given map.
//
// This is intended for unit testing only.
func ProvidersForTesting(schemas map[addrs.Provider]*providers.GetProviderSchemaResponse) Providers {
	return providersStatic{schemas}
}

// ProvisionersForTesting returns a [Provisioners] implementation that just
// returns information directly from the given map.
//
// This is intended for unit testing only.
func ProvisionersForTesting(schemas map[string]*configschema.Block) Provisioners {
	return provisionersStatic{schemas}
}

type externalModulesStatic struct {
	modules map[addrs.ModuleSourceLocal]*configs.Module
}

// ModuleConfig implements ExternalModules.
func (ms externalModulesStatic) ModuleConfig(_ context.Context, source addrs.ModuleSource, _ versions.Set, _ *addrs.AbsModuleCall) (*configs.Module, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	localSource, ok := source.(addrs.ModuleSourceLocal)
	if !ok {
		diags = diags.Append(fmt.Errorf("only local module source addresses are supported for this test"))
		return nil, diags
	}
	ret, ok := ms.modules[localSource]
	if !ok {
		diags = diags.Append(fmt.Errorf("module path %q is not available to this test", localSource))
		return nil, diags
	}
	return ret, diags
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

type provisionersStatic struct {
	schemas map[string]*configschema.Block
}

// ProvisionerConfigSchema implements Provisioners.
func (p provisionersStatic) ProvisionerConfigSchema(_ context.Context, typeName string) (*configschema.Block, tfdiags.Diagnostics) {
	return p.schemas[typeName], nil
}
