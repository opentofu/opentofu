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
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
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
	providers    map[addrs.Provider]providers.Factory
	provisioners map[string]provisioners.Factory

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

func NewRuntimePlugins(providers map[addrs.Provider]providers.Factory, provisioners map[string]provisioners.Factory) Plugins {
	return &newRuntimePlugins{
		providers:    providers,
		provisioners: provisioners,
	}
}

// NewConfiguredProvider implements evalglue.Providers.
func (n *newRuntimePlugins) NewConfiguredProvider(ctx context.Context, provider addrs.Provider, configVal cty.Value) (providers.Configured, tfdiags.Diagnostics) {
	inst, diags := n.newProviderInst(ctx, provider)
	if diags.HasErrors() {
		return nil, diags
	}

	unmarkedConfigVal, _ := configVal.UnmarkDeep()
	resp := inst.ConfigureProvider(ctx, providers.ConfigureProviderRequest{
		Config: unmarkedConfigVal,

		// We aren't actually Terraform, so we'll just pretend to be a
		// Terraform version that has roughly the same functionality that
		// OpenTofu currently has, since providers are permitted to use this to
		// adapt their behavior for older versions of Terraform.
		TerraformVersion: "1.13.0",
	})
	diags = diags.Append(resp.Diagnostics)
	if resp.Diagnostics.HasErrors() {
		return nil, diags
	}

	return inst, diags
}

// ProviderConfigSchema implements evalglue.Providers.
func (n *newRuntimePlugins) ProviderConfigSchema(ctx context.Context, provider addrs.Provider) (*providers.Schema, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	inst, moreDiags := n.unconfiguredProviderInst(ctx, provider)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}

	resp := inst.GetProviderSchema(ctx)
	diags = diags.Append(resp.Diagnostics)
	if resp.Diagnostics.HasErrors() {
		return nil, diags
	}

	return &resp.Provider, diags
}

// ResourceTypeSchema implements evalglue.Providers.
func (n *newRuntimePlugins) ResourceTypeSchema(ctx context.Context, provider addrs.Provider, mode addrs.ResourceMode, typeName string) (*providers.Schema, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	inst, moreDiags := n.unconfiguredProviderInst(ctx, provider)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}

	resp := inst.GetProviderSchema(ctx)
	diags = diags.Append(resp.Diagnostics)
	if resp.Diagnostics.HasErrors() {
		return nil, diags
	}

	// NOTE: Callers expect us to return nil if we successfully fetch the
	// provider schema and then find there is no matching resource type, because
	// the caller is typically in a better position to return a useful error
	// message than we are.

	var types map[string]providers.Schema
	switch mode {
	case addrs.ManagedResourceMode:
		types = resp.ResourceTypes
	case addrs.DataResourceMode:
		types = resp.DataSources
	case addrs.EphemeralResourceMode:
		types = resp.EphemeralResources
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

func (m *newRuntimePlugins) unconfiguredProviderInst(ctx context.Context, provider addrs.Provider) (providers.Unconfigured, tfdiags.Diagnostics) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if running, ok := m.unconfiguredInsts[provider]; ok {
		return running, nil
	}

	inst, diags := m.newProviderInst(ctx, provider)
	if diags.HasErrors() {
		return nil, diags
	}

	if m.unconfiguredInsts == nil {
		m.unconfiguredInsts = make(map[addrs.Provider]providers.Unconfigured)
	}
	m.unconfiguredInsts[provider] = inst
	return inst, diags
}

// newProviderInst creates a new instance of the given provider.
//
// The result is not retained anywhere inside the receiver. Each call to this
// function returns a new object. A successful result is always an unconfigured
// provider, but we return [providers.Interface] in case the caller would like
// to subsequently configure the result before returning it as
// [providers.Configured].
//
// If you intend to use the resulting instance only for "unconfigured"
// operations like fetching schema, use
// [newRuntimePlugins.unconfiguredProviderInst] instead to potentially reuse
// an already-active instance of the same provider.
func (m *newRuntimePlugins) newProviderInst(_ context.Context, provider addrs.Provider) (providers.Interface, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	factory, ok := m.providers[provider]
	if !ok {
		// FIXME: If this error remains reachable in the final version of this
		// code (i.e. if some caller isn't already guaranteeing that all
		// providers from the configuration and state are included here) then
		// we should make this error message more actionable.
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Provider unavailable",
			fmt.Sprintf("This configuration requires provider %q, but it isn't installed.", provider),
		))
		return nil, diags
	}

	inst, err := factory()
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Provider failed to start",
			fmt.Sprintf("Failed to launch provider %q: %s.", provider, tfdiags.FormatError(err)),
		))
		return nil, diags
	}

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

	var errs error
	for addr, p := range n.unconfiguredInsts {
		err := p.Close(ctx)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("closing provider %q: %w", addr, err))
		}
	}
	n.unconfiguredInsts = nil // discard all of the memoized instances
	return errs
}
