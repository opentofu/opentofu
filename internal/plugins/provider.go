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

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
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

type ProviderManager interface {
	HasProvider(addr addrs.Provider) bool

	GetProviderSchema(ctx context.Context, addr addrs.Provider) (providers.ProviderSchema, tfdiags.Diagnostics)

	NewProvider(ctx context.Context, addr addrs.Provider) (providers.Interface, tfdiags.Diagnostics)
	NewConfiguredProvider(ctx context.Context, addr addrs.Provider, cfgVal cty.Value) (providers.Configured, tfdiags.Diagnostics)

	Close(context.Context) error

	Stop(context.Context) error
}

type providerManager struct {
	*library

	instancesLock sync.Mutex
	instances     []providers.Configured

	closed bool
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
	// Coarse lock only for ensuring that a valid entry exists
	p.providerSchemasLock.Lock()
	entry, ok := p.providerSchemas[addr]
	if !ok {
		entry = sync.OnceValue(func() providerSchemaResult {
			log.Printf("[TRACE] plugins.providerManager Initializing provider %q to read its schema", addr)

			var diags tfdiags.Diagnostics

			if p.closed {
				return providerSchemaResult{diags: tfdiags.Diagnostics{}.Append(fmt.Errorf("bug: unable to start provider %s, manager is closed", addr))}
			}

			provider, err := p.providerFactories.NewInstance(addr)
			if err != nil {
				return providerSchemaResult{diags: diags.Append(fmt.Errorf("failed to instantiate provider %q to obtain schema: %w", addr, err))}
			}

			schema := provider.GetProviderSchema(ctx)
			diags = diags.Append(schema.Diagnostics)
			if diags.HasErrors() {
				return providerSchemaResult{schema, diags}
			}

			err = schema.Validate(addr)
			if err != nil {
				diags = diags.Append(err)
			}

			return providerSchemaResult{schema, diags}
		})
		p.providerSchemas[addr] = entry
	}
	// This lock is only for access to the map. We don't need to hold the lock when calling
	// "entry" because [sync.OnceValues] handles synchronization itself.
	// We don't defer unlock as the majority of the work of this function happens in calling "entry"
	// and we want to release as soon as possible for multiple concurrent callers of different providers
	p.providerSchemasLock.Unlock()

	result := entry()
	return result.schema, result.diags
}

func (p *providerManager) NewProvider(ctx context.Context, addr addrs.Provider) (providers.Interface, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	p.instancesLock.Lock()
	defer p.instancesLock.Unlock()
	if p.closed {
		return nil, diags.Append(fmt.Errorf("bug: unable to start provider %s, manager is closed", addr))
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
	if p.closed {
		return nil, diags.Append(fmt.Errorf("bug: unable to start provider %s, manager is closed", addr))
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
		TerraformVersion: "1.13.0",
		Config:           unmarkedConfigVal,
	})
	diags = diags.Append(configResp.Diagnostics)

	return provider, diags
}

func (p *providerManager) Close(ctx context.Context) error {
	p.instancesLock.Lock()
	defer p.instancesLock.Unlock()

	p.closed = true

	var errs []error

	for _, instance := range p.instances {
		errs = append(errs, instance.Close(ctx))
	}

	return errors.Join(errs...)
}

func (p *providerManager) Stop(ctx context.Context) error {
	p.instancesLock.Lock()
	defer p.instancesLock.Unlock()

	var errs []error

	for _, instance := range p.instances {
		errs = append(errs, instance.Stop(ctx))
	}

	return errors.Join(errs...)
}
