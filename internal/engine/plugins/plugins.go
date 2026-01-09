// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugins

import (
	"context"
	"errors"
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plugins"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

type Plugins interface {
	Providers
	Provisioners
}

// Providers is implemented by callers of this package to provide access
// to the providers needed by a configuration without this package needing
// to know anything about how provider plugins work, or whether plugins are
// even being used.
type Providers interface {
	eval.ProvidersSchema

	// ValidateProviderConfig runs provider-specific logic to check whether
	// the given configuration is valid. Returns at least one error diagnostic
	// if the configuration is not valid, and may also return warning
	// diagnostics regardless of whether the configuration is valid.
	//
	// The given config value is guaranteed to be an object conforming to
	// the schema returned by a previous call to ProviderConfigSchema for
	// the same provider.
	ValidateProviderConfig(ctx context.Context, provider addrs.Provider, configVal cty.Value) tfdiags.Diagnostics

	// ValidateResourceConfig runs provider-specific logic to check whether
	// the given configuration is valid. Returns at least one error diagnostic
	// if the configuration is not valid, and may also return warning
	// diagnostics regardless of whether the configuration is valid.
	//
	// The given config value is guaranteed to be an object conforming to
	// the schema returned by a previous call to ResourceTypeSchema for
	// the same resource type.
	ValidateResourceConfig(ctx context.Context, provider addrs.Provider, mode addrs.ResourceMode, typeName string, configVal cty.Value) tfdiags.Diagnostics

	// NewConfiguredProvider starts a _configured_ instance of the given
	// provider using the given configuration value.
	//
	// The evaluation system itself makes no use of configured providers, but
	// higher-level processes wrapping it (e.g. the plan and apply engines)
	// need to use configured providers for actions related to resources, etc,
	// and so this is for their benefit to help ensure that they are definitely
	// creating a configured instance of the same provider that other methods
	// would be using to return schema information and validation results.
	//
	// It's the caller's responsibility to ensure that the given configuration
	// value is valid according to the provider's schema and validation rules.
	// That's usually achieved by taking a value provided by the evaluation
	// system, which would then have already been processed using the results
	// from [Providers.ProviderConfigSchema] and
	// [Providers.ValidateProviderConfig]. If the returned diagnostics contains
	// errors then the [providers.Configured] result is invalid and must not be
	// used.
	NewConfiguredProvider(ctx context.Context, provider addrs.Provider, configVal cty.Value) (providers.Configured, tfdiags.Diagnostics)

	Close(ctx context.Context) error
}

type Provisioners interface {
	eval.ProvisionersSchema
}

type newRuntimePlugins struct {
	providers    plugins.ProviderManager
	provisioners plugins.ProvisionerManager
}

var _ Providers = (*newRuntimePlugins)(nil)
var _ Provisioners = (*newRuntimePlugins)(nil)

func NewRuntimePlugins(plugins plugins.Library) Plugins {
	return &newRuntimePlugins{
		providers:    plugins.NewProviderManager(),
		provisioners: plugins.NewProvisionerManager(),
	}
}

// NewConfiguredProvider implements evalglue.Providers.
func (n *newRuntimePlugins) NewConfiguredProvider(ctx context.Context, provider addrs.Provider, configVal cty.Value) (providers.Configured, tfdiags.Diagnostics) {
	// TODO addr
	return n.providers.NewConfiguredProvider(ctx, addrs.AbsProviderInstanceCorrect{
		Config: addrs.AbsProviderConfigCorrect{
			Config: addrs.ProviderConfigCorrect{
				Provider: provider,
			},
		},
	}, configVal)
}

// ProviderConfigSchema implements evalglue.Providers.
func (n *newRuntimePlugins) ProviderConfigSchema(ctx context.Context, provider addrs.Provider) (*providers.Schema, tfdiags.Diagnostics) {
	return n.providers.ProviderConfigSchema(ctx, provider)
}

// ResourceTypeSchema implements evalglue.Providers.
func (n *newRuntimePlugins) ResourceTypeSchema(ctx context.Context, provider addrs.Provider, mode addrs.ResourceMode, typeName string) (*providers.Schema, tfdiags.Diagnostics) {
	return n.providers.ResourceTypeSchema(ctx, provider, mode, typeName)
}

// ValidateProviderConfig implements evalglue.Providers.
func (n *newRuntimePlugins) ValidateProviderConfig(ctx context.Context, provider addrs.Provider, configVal cty.Value) tfdiags.Diagnostics {
	return n.providers.ValidateProviderConfig(ctx, provider, configVal)
}

// ValidateResourceConfig implements evalglue.Providers.
func (n *newRuntimePlugins) ValidateResourceConfig(ctx context.Context, provider addrs.Provider, mode addrs.ResourceMode, typeName string, configVal cty.Value) tfdiags.Diagnostics {
	return n.providers.ValidateResourceConfig(ctx, provider, mode, typeName, configVal)
}

// ProvisionerConfigSchema implements evalglue.Provisioners.
func (n *newRuntimePlugins) ProvisionerConfigSchema(ctx context.Context, typeName string) (*configschema.Block, tfdiags.Diagnostics) {
	// TODO: Implement this in terms of [newRuntimePlugins.provisioners].
	// But provisioners aren't in scope for our "walking skeleton" phase of
	// development, so we'll skip this for now.
	var diags tfdiags.Diagnostics
	diags = diags.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"Cannot use providers in new runtime codepath",
		fmt.Sprintf("Can't use provisioner %q: new runtime codepath doesn't know how to instantiate provisioners yet", typeName),
	))
	return nil, diags
}

// Close terminates any plugins that are managed by this object and are still
// running.
func (n *newRuntimePlugins) Close(ctx context.Context) error {
	return errors.Join(n.providers.Close(ctx), n.provisioners.Close())
}
