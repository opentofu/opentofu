// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugins

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/version"
	"github.com/zclconf/go-cty/cty"
)

type ProviderFactories map[addrs.Provider]providers.Factory

func (p ProviderFactories) HasProvider(addr addrs.Provider) bool {
	_, ok := p[addr]
	return ok
}

func (p ProviderFactories) NewInstance(addr addrs.Provider) (providers.Interface, error) {
	f, ok := p[addr]
	if !ok {
		return nil, fmt.Errorf("unavailable provider %q", addr)
	}

	return f()
}

// ProviderManager allows for spawning, tracking and management of provider instances.
type ProviderManager interface {
	// HasProvider checks to see if the underlying library contains a given provider.
	HasProvider(addr addrs.Provider) bool

	// GetProviderSchema returns a fully validated and cached provider schema. This should
	// always be preferred to accessing the schema directly from a provider.
	GetProviderSchema(ctx context.Context, addr addrs.Provider) (providers.ProviderSchema, tfdiags.Diagnostics)

	// NewProvider starts and tracks a new provider instance of the given type.
	NewProvider(ctx context.Context, addr addrs.Provider) (providers.Interface, tfdiags.Diagnostics)
	// NewConfiguredProvider starts, configures, and tracks a new provider instance of the give type.
	NewConfiguredProvider(ctx context.Context, addr addrs.Provider, cfgVal cty.Value) (providers.Configured, tfdiags.Diagnostics)

	// StopAll gracefully requests all tracked providers to stop.
	// See [providers.Unconfigured.Stop] for more information.
	StopAll(context.Context) error
	// CloseAll forcefully closes all tracked providers.
	// See [providers.Unconfigured.Close] for more information.
	// See cmd/tofu/main.go:plugin.CleanupClients for the fallback.
	CloseAll(context.Context) error
	// Shutdown locks the provider manager in a Shutdown state and calls CloseAll,
	// preventing any further usage of this object.
	Shutdown(context.Context) error
}

type providerManager struct {
	*library

	instancesLock sync.Mutex
	instances     []providers.Configured

	isShutdown atomic.Bool
}

func (l *library) NewProviderManager() ProviderManager {
	return &providerManager{
		library: l,
	}
}

func (p *providerManager) HasProvider(addr addrs.Provider) bool {
	return p.providerFactories.HasProvider(addr)
}

func (p *providerManager) GetProviderSchema(ctx context.Context, addr addrs.Provider) (providers.ProviderSchema, tfdiags.Diagnostics) {
	if p.isShutdown.Load() {
		// It's technically possible, but highly unlikely that a manager could be shutdown while fetching the schema
		// In that scenario, we will start and then stop the corresponding provider internally to this function and not
		// interfere with the set of known instances.
		return providers.ProviderSchema{}, tfdiags.Diagnostics{}.Append(fmt.Errorf("bug: unable to start provider %s, manager is shutdown", addr))
	}

	// Coarse lock only for ensuring that a valid entry exists
	p.providerSchemasLock.Lock()
	entry, ok := p.providerSchemas[addr]
	if !ok {
		entry = &providerSchemaEntry{}
		p.providerSchemas[addr] = entry
	}
	// This lock is only for access to the map. We don't need to hold the lock when updating the entry
	// because we lock the individual entry for all access.
	// We don't defer unlock as the majority of the work of this function happens in updating the entry
	// and we want to release as soon as possible for multiple concurrent callers of different providers
	p.providerSchemasLock.Unlock()

	entry.Lock()
	defer entry.Unlock()

	if !entry.populated {
		log.Printf("[TRACE] plugins.providerManager Initializing provider %q to read its schema", addr)

		provider, err := p.providerFactories.NewInstance(addr)
		if err != nil {
			// Might be a transient error. Don't memoize this result
			return providers.ProviderSchema{}, tfdiags.Diagnostics{}.Append(fmt.Errorf("failed to instantiate provider %q to obtain schema: %w", addr, err))
		}
		// TODO consider using the p.NewProvider(ctx, addr) call once we have a clear
		// .Close() call for all usages of the provider manager
		defer provider.Close(context.WithoutCancel(ctx))

		entry.schema = provider.GetProviderSchema(ctx)
		entry.diags = entry.diags.Append(entry.schema.Diagnostics)
		entry.populated = true

		if !entry.diags.HasErrors() {
			// Validate only if GetProviderSchema succeeded
			err := entry.schema.Validate(addr)
			entry.diags = entry.diags.Append(err)
		}
	}

	return entry.schema, entry.diags
}

func (p *providerManager) NewProvider(ctx context.Context, addr addrs.Provider) (providers.Interface, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	p.instancesLock.Lock()
	defer p.instancesLock.Unlock()
	if p.isShutdown.Load() {
		return nil, diags.Append(fmt.Errorf("bug: unable to start provider %s, manager is shutdown", addr))
	}

	provider, err := p.providerFactories.NewInstance(addr)
	if err != nil {
		return nil, diags.Append(err)
	}

	p.instances = append(p.instances, provider)

	return provider, diags
}

func (p *providerManager) NewConfiguredProvider(ctx context.Context, addr addrs.Provider, configVal cty.Value) (providers.Configured, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	p.instancesLock.Lock()
	defer p.instancesLock.Unlock()
	if p.isShutdown.Load() {
		return nil, diags.Append(fmt.Errorf("bug: unable to start provider %s, manager is shutdown", addr))
	}

	provider, err := p.providerFactories.NewInstance(addr)
	if err != nil {
		return nil, diags.Append(err)
	}

	p.instances = append(p.instances, provider)

	// If our config value contains any marked values, ensure those are
	// stripped out before sending this to the provider
	unmarkedConfigVal, _ := configVal.UnmarkDeep()

	// Allow the provider to validate and insert any defaults into the full
	// configuration.
	req := providers.ValidateProviderConfigRequest{
		Config: unmarkedConfigVal,
	}

	// ValidateProviderConfig is only used for validation. We are intentionally
	// ignoring the PreparedConfig field to maintain existing behavior.
	validateResp := provider.ValidateProviderConfig(ctx, req)
	diags = diags.Append(validateResp.Diagnostics)
	if diags.HasErrors() {
		return nil, diags
	}

	// If the provider returns something different, log a warning to help
	// indicate to provider developers that the value is not used.
	preparedCfg := validateResp.PreparedConfig
	if preparedCfg != cty.NilVal && !preparedCfg.IsNull() && !preparedCfg.RawEquals(unmarkedConfigVal) {
		log.Printf("[WARN] ValidateProviderConfig from %q changed the config value, but that value is unused", addr)
	}

	configResp := provider.ConfigureProvider(ctx, providers.ConfigureProviderRequest{
		// We aren't actually Terraform, so we'll just pretend to be a
		// Terraform version that has roughly the same functionality that
		// OpenTofu currently has, since providers are permitted to use this to
		// adapt their behavior for older versions of Terraform.
		TerraformVersion: version.VersionToImpersonateForProviders,
		Config:           unmarkedConfigVal,
	})
	diags = diags.Append(configResp.Diagnostics)

	return provider, diags
}

func (p *providerManager) StopAll(ctx context.Context) error {
	p.instancesLock.Lock()
	defer p.instancesLock.Unlock()

	var errs []error

	for _, instance := range p.instances {
		errs = append(errs, instance.Stop(ctx))
	}

	return errors.Join(errs...)
}

func (p *providerManager) CloseAll(ctx context.Context) error {
	p.instancesLock.Lock()
	defer p.instancesLock.Unlock()

	var errs []error

	for _, instance := range p.instances {
		errs = append(errs, instance.Close(ctx))
	}

	return errors.Join(errs...)
}

func (p *providerManager) Shutdown(ctx context.Context) error {
	// Disable any further usage of this manager
	p.isShutdown.Store(true)

	return p.CloseAll(ctx)
}
