// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugins

import (
	"context"
	"errors"
	"fmt"
	"sync"

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
	eval.Providers

	Close(ctx context.Context) error
}

type Provisioners interface {
	eval.ProvisionersSchema
}

type newRuntimePlugins struct {
	providers    plugins.ProviderManager
	provisioners plugins.ProvisionerManager

	// unconfiguredInsts is all of the provider instances we've created for
	// unconfigured uses such as schema fetching and validation, which we
	// currently just leave running for the remainder of the life of this
	// object though perhaps we'll do something more clever eventually.
	//
	// Must hold a lock on mu throughout any access to this map.
	unconfiguredInsts map[addrs.Provider]providers.Unconfigured
	mu                sync.Mutex
}

var _ Providers = (*newRuntimePlugins)(nil)
var _ Provisioners = (*newRuntimePlugins)(nil)

func NewRuntimePluginsTemp(providerManager plugins.ProviderManager, provisionerManager plugins.ProvisionerManager) Plugins {
	return &newRuntimePlugins{
		providers:         providerManager,
		provisioners:      provisionerManager,
		unconfiguredInsts: map[addrs.Provider]providers.Unconfigured{},
	}
}

func NewRuntimePlugins(plugins plugins.Library) Plugins {
	return &newRuntimePlugins{
		providers:         plugins.NewProviderManager(),
		provisioners:      plugins.NewProvisionerManager(),
		unconfiguredInsts: map[addrs.Provider]providers.Unconfigured{},
	}
}

// NewConfiguredProvider implements evalglue.Providers.
func (n *newRuntimePlugins) NewConfiguredProvider(ctx context.Context, provider addrs.Provider, configVal cty.Value) (providers.Configured, tfdiags.Diagnostics) {
	return n.providers.NewConfiguredProvider(ctx, provider, configVal)
}

// ProviderConfigSchema implements evalglue.Providers.
func (n *newRuntimePlugins) ProviderConfigSchema(ctx context.Context, provider addrs.Provider) (*providers.Schema, tfdiags.Diagnostics) {
	schema, diags := n.providers.GetProviderSchema(ctx, provider)
	if diags.HasErrors() {
		return nil, diags
	}

	return &schema.Provider, diags
}

// ResourceTypeSchema implements evalglue.Providers.
func (n *newRuntimePlugins) ResourceTypeSchema(ctx context.Context, provider addrs.Provider, mode addrs.ResourceMode, typeName string) (*providers.Schema, tfdiags.Diagnostics) {
	schema, diags := n.providers.GetProviderSchema(ctx, provider)
	if diags.HasErrors() {
		return nil, diags
	}

	// NOTE: Callers expect us to return nil if we successfully fetch the
	// provider schema and then find there is no matching resource type, because
	// the caller is typically in a better position to return a useful error
	// message than we are.

	var types map[string]providers.Schema
	switch mode {
	case addrs.ManagedResourceMode:
		types = schema.ResourceTypes
	case addrs.DataResourceMode:
		types = schema.DataSources
	case addrs.EphemeralResourceMode:
		types = schema.EphemeralResources
	default:
		// We don't support any other modes, so we'll just treat these as
		// a request for a resource type that doesn't exist at all.
		return nil, nil
	}
	ret, ok := types[typeName]
	if !ok {
		return nil, diags
	}
	return &ret, diags
}

// ValidateProviderConfig implements evalglue.Providers.
func (n *newRuntimePlugins) ValidateProviderConfig(ctx context.Context, provider addrs.Provider, configVal cty.Value) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	inst, moreDiags := n.unconfiguredProviderInst(ctx, provider)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return diags
	}

	unmarkedConfigVal, _ := configVal.UnmarkDeep()
	resp := inst.ValidateProviderConfig(ctx, providers.ValidateProviderConfigRequest{
		Config: unmarkedConfigVal,
	})
	diags = diags.Append(resp.Diagnostics)
	return diags
}

// ValidateResourceConfig implements evalglue.Providers.
func (n *newRuntimePlugins) ValidateResourceConfig(ctx context.Context, provider addrs.Provider, mode addrs.ResourceMode, typeName string, configVal cty.Value) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	inst, moreDiags := n.unconfiguredProviderInst(ctx, provider)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return diags
	}

	switch mode {
	case addrs.ManagedResourceMode:
		resp := inst.ValidateResourceConfig(ctx, providers.ValidateResourceConfigRequest{
			TypeName: typeName,
			Config:   configVal,
		})
		diags = diags.Append(resp.Diagnostics)
	case addrs.DataResourceMode:
		resp := inst.ValidateDataResourceConfig(ctx, providers.ValidateDataResourceConfigRequest{
			TypeName: typeName,
			Config:   configVal,
		})
		diags = diags.Append(resp.Diagnostics)
	case addrs.EphemeralResourceMode:
		resp := inst.ValidateEphemeralConfig(ctx, providers.ValidateEphemeralConfigRequest{
			TypeName: typeName,
			Config:   configVal,
		})
		diags = diags.Append(resp.Diagnostics)
	default:
		// If we get here then it's a bug because the cases above should
		// cover all valid values of [addrs.ResourceMode].
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Unsupported resource mode",
			fmt.Sprintf("Attempted to validate resource of unsupported mode %s; this is a bug in OpenTofu.", mode),
		))
	}
	return diags
}

func (n *newRuntimePlugins) unconfiguredProviderInst(ctx context.Context, provider addrs.Provider) (providers.Unconfigured, tfdiags.Diagnostics) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if running, ok := n.unconfiguredInsts[provider]; ok {
		return running, nil
	}

	inst, diags := n.providers.NewProvider(ctx, provider)
	if diags.HasErrors() {
		return nil, diags
	}

	if n.unconfiguredInsts == nil {
		n.unconfiguredInsts = make(map[addrs.Provider]providers.Unconfigured)
	}
	n.unconfiguredInsts[provider] = inst
	return inst, diags
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
	n.mu.Lock()
	defer n.mu.Unlock()

	n.unconfiguredInsts = nil // discard all of the memoized instances
	return errors.Join(n.providers.CloseAll(ctx), n.provisioners.CloseAll())
}
