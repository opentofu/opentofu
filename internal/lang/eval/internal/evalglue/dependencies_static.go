// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package evalglue

import (
	"context"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

type staticPlugins struct{}

var _ Providers = staticPlugins{}
var _ Provisioners = staticPlugins{}

func NewStaticPlugins() staticPlugins {
	return staticPlugins{}
}

var StaticProviderSchema = &providers.Schema{}
var StaticProvisionerSchema = &configschema.Block{}

func (s staticPlugins) ProviderConfigSchema(ctx context.Context, provider addrs.Provider) (*providers.Schema, tfdiags.Diagnostics) {
	return StaticProviderSchema, nil
}
func (s staticPlugins) ResourceTypeSchema(ctx context.Context, provider addrs.Provider, mode addrs.ResourceMode, typeName string) (*providers.Schema, tfdiags.Diagnostics) {
	return StaticProviderSchema, nil
}
func (s staticPlugins) ValidateProviderConfig(ctx context.Context, provider addrs.Provider, configVal cty.Value) tfdiags.Diagnostics {
	return nil
}
func (s staticPlugins) ValidateResourceConfig(ctx context.Context, provider addrs.Provider, mode addrs.ResourceMode, typeName string, configVal cty.Value) tfdiags.Diagnostics {
	return nil
}
func (s staticPlugins) BuildFunction(ctx context.Context, provider addrs.Provider, pf addrs.ProviderFunction, stubMissing bool, rng hcl.Range) (function.Function, tfdiags.Diagnostics) {
	panic("not supported in a static context")
}
func (s staticPlugins) NewConfiguredProvider(ctx context.Context, provider addrs.Provider, configVal cty.Value) (providers.Configured, tfdiags.Diagnostics) {
	panic("not supported in a static context")
}
func (s staticPlugins) ProvisionerConfigSchema(ctx context.Context, typeName string) (*configschema.Block, tfdiags.Diagnostics) {
	return StaticProvisionerSchema, nil
}
func (s staticPlugins) ValidateProvisionerConfig(ctx context.Context, typ string, config cty.Value) tfdiags.Diagnostics {
	return nil
}
