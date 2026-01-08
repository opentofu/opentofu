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
	"time"

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

type ProviderManager interface {
	HasProvider(addr addrs.Provider) bool

	GetProviderSchema(ctx context.Context, addr addrs.Provider) (providers.ProviderSchema, tfdiags.Diagnostics)

	Unconfigured(addr addrs.Provider, fn func(providers.Unconfigured) tfdiags.Diagnostics) tfdiags.Diagnostics

	NewProvider(ctx context.Context, addr addrs.Provider) (providers.Interface, tfdiags.Diagnostics)

	NewConfiguredProvider(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, cfgVal cty.Value) (providers.Configured, tfdiags.Diagnostics)

	Close(context.Context) error

	Stop(context.Context) error
}

type providerManager struct {
	*library

	instancesLock sync.Mutex
	instances     map[addrs.Provider]*unconfiguredProvider

	configuredInstances []providers.Configured

	closed bool
}

type unconfiguredProvider struct {
	providers.Unconfigured
	sync.Mutex

	lastUsed time.Time
	active   atomic.Int32
}

func (l *library) NewProviderManager() ProviderManager {
	return &providerManager{
		library:   l,
		instances: map[addrs.Provider]*unconfiguredProvider{},
	}
}

func (p *providerManager) HasProvider(addr addrs.Provider) bool {
	return p.providerFactories.HasProvider(addr)
}

func (p *providerManager) unconfigured(addr addrs.Provider) (providers.Unconfigured, func(), error) {
	p.instancesLock.Lock()
	if p.closed {
		return nil, func() {}, fmt.Errorf("bug: unable to start provider %s, manager is closed", addr)
	}

	instance, ok := p.instances[addr]
	if !ok {
		// Setup entry
		instance = &unconfiguredProvider{}
		p.instances[addr] = instance
	}
	p.instancesLock.Unlock()

	// Reduce lock scope to specific instance
	instance.Lock()
	defer instance.Unlock()

	if instance.Unconfigured == nil {
		// Try to startup the instance
		inst, err := p.providerFactories.NewInstance(addr)
		if err != nil {
			return nil, func() {}, err
		}

		// Setup instance with constructed
		instance.Unconfigured = inst
		instance.lastUsed = time.Now()

		// Start shutdown watcher for this inst
		go func() {
			expiration := time.Duration(5 * time.Second)
			for time.Now().Before(instance.lastUsed.Add(expiration)) || instance.active.Load() != 0 {
				time.Sleep(expiration)
			}
			// Shutdown (could be racey)
			instance.Lock()
			defer instance.Unlock()
			if instance.Unconfigured != nil {
				err := instance.Stop(context.TODO())
				if err != nil {
					log.Printf("[WARN] Unable to stop provider %s: %s", addr, err.Error())
				}
				instance.Unconfigured = nil
			}
		}()

	}

	// Mark the instance as active and currently in use
	instance.lastUsed = time.Now()
	instance.active.Add(1)

	return instance.Unconfigured, func() {
		instance.lastUsed = time.Now()
		instance.active.Add(-1)
	}, nil
}

func (p *providerManager) GetProviderSchema(ctx context.Context, addr addrs.Provider) (providers.ProviderSchema, tfdiags.Diagnostics) {
	// Coarse lock only for ensuring that a valid entry exists
	p.providerSchemasLock.Lock()
	entry, ok := p.providerSchemas[addr]
	if !ok {
		entry = sync.OnceValue(func() providerSchemaResult {
			log.Printf("[TRACE] tofu.contextPlugins: Initializing provider %q to read its schema", addr)

			var result providerSchemaResult

			result.diags = p.Unconfigured(addr, func(provider providers.Unconfigured) tfdiags.Diagnostics {
				result.schema = provider.GetProviderSchema(ctx)
				return nil
			})

			result.diags = result.diags.Append(result.schema.Diagnostics)

			if result.diags.HasErrors() {
				return result
			}

			err := result.schema.Validate(addr)
			if err != nil {
				result.diags = result.diags.Append(err)
			}

			return result
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

func (p *providerManager) Unconfigured(addr addrs.Provider, fn func(providers.Unconfigured) tfdiags.Diagnostics) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	inst, done, err := p.unconfigured(addr)
	defer done()

	if err != nil {
		return diags.Append(err)
	}
	return diags.Append(fn(inst))
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

	p.configuredInstances = append(p.configuredInstances, provider)

	return provider, diags
}

func (p *providerManager) NewConfiguredProvider(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, configVal cty.Value) (providers.Configured, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	p.instancesLock.Lock()
	defer p.instancesLock.Unlock()
	if p.closed {
		return nil, diags.Append(fmt.Errorf("bug: unable to start provider %s, manager is closed", addr))
	}

	provider, err := p.providerFactories.NewInstance(addr.Config.Config.Provider)
	if err != nil {
		return nil, diags.Append(err)
	}

	p.configuredInstances = append(p.configuredInstances, provider)

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
		TerraformVersion: version.String(),
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
		instance.Lock()
		defer instance.Unlock()

		errs = append(errs, instance.Close(ctx))

		instance.Unconfigured = nil
	}

	for _, instance := range p.configuredInstances {
		errs = append(errs, instance.Close(ctx))
	}

	return errors.Join(errs...)
}

func (p *providerManager) Stop(ctx context.Context) error {
	p.instancesLock.Lock()
	defer p.instancesLock.Unlock()

	var errs []error

	for _, instance := range p.instances {
		instance.Lock()
		defer instance.Unlock()

		errs = append(errs, instance.Stop(ctx))

		instance.Unconfigured = nil
	}

	for _, instance := range p.configuredInstances {
		errs = append(errs, instance.Stop(ctx))
	}

	return errors.Join(errs...)
}
