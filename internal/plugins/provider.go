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

	GetProviderSchema(ctx context.Context, addr addrs.Provider) providers.ProviderSchema

	// ProviderConfigSchema returns the schema that should be used to evaluate
	// a "provider" block associated with the given provider.
	//
	// All providers are required to have a config schema, although for some
	// providers it is completely empty to represent that no explicit
	// configuration is needed.
	ProviderConfigSchema(ctx context.Context, addr addrs.Provider) (*providers.Schema, tfdiags.Diagnostics)

	// ResourceTypeSchema returns the schema for configuration and state of
	// a resource of the given type, or nil if the given provider does not
	// offer any such resource type.
	//
	// Returns error diagnostics if the given provider isn't available for use
	// at all, regardless of the resource type.
	ResourceTypeSchema(ctx context.Context, addr addrs.Provider, mode addrs.ResourceMode, typeName string) (*providers.Schema, tfdiags.Diagnostics)

	// ValidateProviderConfig runs provider-specific logic to check whether
	// the given configuration is valid. Returns at least one error diagnostic
	// if the configuration is not valid, and may also return warning
	// diagnostics regardless of whether the configuration is valid.
	//
	// The given config value is guaranteed to be an object conforming to
	// the schema returned by a previous call to ProviderConfigSchema for
	// the same provider.
	ValidateProviderConfig(ctx context.Context, addr addrs.Provider, cfgVal cty.Value) tfdiags.Diagnostics

	// ValidateResourceConfig runs provider-specific logic to check whether
	// the given configuration is valid. Returns at least one error diagnostic
	// if the configuration is not valid, and may also return warning
	// diagnostics regardless of whether the configuration is valid.
	//
	// The given config value is guaranteed to be an object conforming to
	// the schema returned by a previous call to ResourceTypeSchema for
	// the same resource type.
	ValidateResourceConfig(ctx context.Context, addr addrs.Provider, mode addrs.ResourceMode, typeName string, cfgVal cty.Value) tfdiags.Diagnostics

	MoveResourceState(ctx context.Context, addr addrs.Provider, req providers.MoveResourceStateRequest) providers.MoveResourceStateResponse

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

func (p *providerManager) GetProviderSchema(ctx context.Context, addr addrs.Provider) providers.ProviderSchema {
	// Coarse lock only for ensuring that a valid entry exists
	p.providerSchemasLock.Lock()
	entry, ok := p.providerSchemas[addr]
	if !ok {
		entry = sync.OnceValue(func() providers.ProviderSchema {
			log.Printf("[TRACE] tofu.contextPlugins: Initializing provider %q to read its schema", addr)
			provider, done, err := p.unconfigured(addr)
			defer done()
			if err != nil {
				return providers.ProviderSchema{
					Diagnostics: tfdiags.Diagnostics{}.Append(
						fmt.Errorf("failed to instantiate provider %q to obtain schema: %w", addr, err),
					)}
			}

			schema := provider.GetProviderSchema(ctx)

			err = schema.Validate(addr)
			if err != nil {
				schema.Diagnostics = schema.Diagnostics.Append(err)
			}

			return schema
		})
		p.providerSchemas[addr] = entry
	}
	// This lock is only for access to the map. We don't need to hold the lock when calling
	// "entry" because [sync.OnceValues] handles synchronization itself.
	// We don't defer unlock as the majority of the work of this function happens in calling "entry"
	// and we want to release as soon as possible for multiple concurrent callers of different providers
	p.providerSchemasLock.Unlock()

	return entry()
}

// ProviderConfigSchema returns the schema that should be used to evaluate
// a "provider" block associated with the given provider.
//
// All providers are required to have a config schema, although for some
// providers it is completely empty to represent that no explicit
// configuration is needed.
func (p *providerManager) ProviderConfigSchema(ctx context.Context, addr addrs.Provider) (*providers.Schema, tfdiags.Diagnostics) {
	schema := p.GetProviderSchema(ctx, addr)
	diags := schema.Diagnostics

	if diags.HasErrors() {
		return nil, diags
	}

	return &schema.Provider, diags
}

// ResourceTypeSchema returns the schema for configuration and state of
// a resource of the given type, or nil if the given provider does not
// offer any such resource type.
//
// Returns error diagnostics if the given provider isn't available for use
// at all, regardless of the resource type.
func (p *providerManager) ResourceTypeSchema(ctx context.Context, addr addrs.Provider, mode addrs.ResourceMode, typeName string) (*providers.Schema, tfdiags.Diagnostics) {
	schema := p.GetProviderSchema(ctx, addr)
	diags := schema.Diagnostics

	if diags.HasErrors() {
		return nil, diags
	}

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
		return nil, diags
	}
	ret, ok := types[typeName]
	if !ok {
		return nil, diags
	}

	// TODO ret.Block == nil error
	/*
		schema, currentVersion := (providerSchema).SchemaForResourceAddr(addr.Resource.ContainingResource())
		if schema == nil {
			// Shouldn't happen since we should've failed long ago if no schema is present
			return nil, diags.Append(fmt.Errorf("no schema available for %s while reading state; this is a bug in OpenTofu and should be reported", addr))
		}*/

	return &ret, diags
}

// ValidateProviderConfig runs provider-specific logic to check whether
// the given configuration is valid. Returns at least one error diagnostic
// if the configuration is not valid, and may also return warning
// diagnostics regardless of whether the configuration is valid.
//
// The given config value is guaranteed to be an object conforming to
// the schema returned by a previous call to ProviderConfigSchema for
// the same provider.
func (p *providerManager) ValidateProviderConfig(ctx context.Context, addr addrs.Provider, cfgVal cty.Value) tfdiags.Diagnostics {
	cfgVal, _ = cfgVal.UnmarkDeep()

	inst, done, err := p.unconfigured(addr)
	defer done()
	if err != nil {
		return tfdiags.Diagnostics{}.Append(err)
	}

	// NOTE: we ignore resp.PreparedConfig in this codepath, but not below in ConfigureProvider
	// This is to handle some oddities in tfplugin5, documented in providers.ValidateProviderConfigResponse
	return inst.ValidateProviderConfig(ctx, providers.ValidateProviderConfigRequest{Config: cfgVal}).Diagnostics
}

// ValidateResourceConfig runs provider-specific logic to check whether
// the given configuration is valid. Returns at least one error diagnostic
// if the configuration is not valid, and may also return warning
// diagnostics regardless of whether the configuration is valid.
//
// The given config value is guaranteed to be an object conforming to
// the schema returned by a previous call to ResourceTypeSchema for
// the same resource type.
func (p *providerManager) ValidateResourceConfig(ctx context.Context, addr addrs.Provider, mode addrs.ResourceMode, typeName string, cfgVal cty.Value) tfdiags.Diagnostics {
	cfgVal, _ = cfgVal.UnmarkDeep()

	inst, done, err := p.unconfigured(addr)
	defer done()
	if err != nil {
		return tfdiags.Diagnostics{}.Append(err)
	}

	switch mode {
	case addrs.ManagedResourceMode:
		return inst.ValidateResourceConfig(ctx, providers.ValidateResourceConfigRequest{
			TypeName: typeName,
			Config:   cfgVal,
		}).Diagnostics
	case addrs.DataResourceMode:
		return inst.ValidateDataResourceConfig(ctx, providers.ValidateDataResourceConfigRequest{
			TypeName: typeName,
			Config:   cfgVal,
		}).Diagnostics
	case addrs.EphemeralResourceMode:
		return inst.ValidateEphemeralConfig(ctx, providers.ValidateEphemeralConfigRequest{
			TypeName: typeName,
			Config:   cfgVal,
		}).Diagnostics
	default:
		// We don't support any other modes, so we'll just treat these as
		// a request for a resource type that doesn't exist at all.
		return nil
	}
}

func (p *providerManager) MoveResourceState(ctx context.Context, addr addrs.Provider, req providers.MoveResourceStateRequest) providers.MoveResourceStateResponse {
	inst, done, err := p.unconfigured(addr)
	defer done()
	if err != nil {
		return providers.MoveResourceStateResponse{Diagnostics: tfdiags.Diagnostics{}.Append(err)}
	}
	return inst.MoveResourceState(ctx, req)
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

	p.closed = true

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
