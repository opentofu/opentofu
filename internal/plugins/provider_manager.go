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
	"github.com/zclconf/go-cty/cty"
)

type providerManager struct {
	factories map[addrs.Provider]providers.Factory

	schemasLock sync.Mutex
	schemas     map[addrs.Provider]ProviderSchema

	unconfiguredLock sync.Mutex
	unconfigured     map[addrs.Provider]*providerUnconfigured

	configuredLock sync.Mutex
	configured     map[string]providers.Configured
}

type providerUnconfigured struct {
	providers.Unconfigured

	access time.Time
	active atomic.Int32
}

func NewProviderMananger(ctx context.Context, factories map[addrs.Provider]providers.Factory) ProviderManager {
	manager := &providerManager{
		factories: factories,

		unconfigured: map[addrs.Provider]*providerUnconfigured{},
		configured:   map[string]providers.Configured{},
	}

	go func() {
		// TODO configurable
		expiration := time.Duration(15 * time.Second)
		for {
			manager.unconfiguredLock.Lock()
			for addr, entry := range manager.unconfigured {
				if entry.active.Load() == 0 && entry.access.Before(time.Now().Add(expiration)) {
					// Not used recently and not active
					err := entry.Stop(ctx)
					if err != nil {
						// This is not ideal
						log.Printf("[ERROR] Unable to stop provider %s: %q", addr, err.Error())
					}

					delete(manager.unconfigured, addr)
				}
			}
			manager.unconfiguredLock.Unlock()

			select {
			case <-time.After(expiration):
				continue
			case <-ctx.Done():
				break
			}
		}

		err := manager.Stop(ctx)
		if err != nil {
			log.Printf("[ERROR] Unable to stop provider manager: %s", err.Error())
		}
	}()

	return manager
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
// [manager.unconfigured] instead to potentially reuse
// an already-active instance of the same provider.
func (p *providerManager) newProviderInst(ctx context.Context, addr addrs.Provider) (providers.Interface, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	factory, ok := p.factories[addr]
	if !ok {
		// FIXME: If this error remains reachable in the final version of this
		// code (i.e. if some caller isn't already guaranteeing that all
		// providers from the configuration and state are included here) then
		// we should make this error message more actionable.
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Provider unavailable",
			fmt.Sprintf("This configuration requires provider %q, but it isn't installed.", addr),
		))
		return nil, diags
	}

	inst, err := factory()
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Provider failed to start",
			fmt.Sprintf("Failed to launch provider %q: %s.", addr, tfdiags.FormatError(err)),
		))
		return nil, diags
	}

	return inst, diags
}

func (p *providerManager) unconfiguredProvider(ctx context.Context, addr addrs.Provider) (providers.Unconfigured, func(), tfdiags.Diagnostics) {
	p.unconfiguredLock.Lock()
	defer p.unconfiguredLock.Unlock()

	instance, ok := p.unconfigured[addr]
	if !ok {
		inst, diags := p.newProviderInst(ctx, addr)
		if diags.HasErrors() {
			return nil, func() {}, diags
		}

		instance = &providerUnconfigured{Unconfigured: inst}
		p.unconfigured[addr] = instance
	}

	instance.access = time.Now()
	instance.active.Add(1)

	return instance.Unconfigured, func() {
		instance.access = time.Now()
		instance.active.Add(-1)
	}, nil
}

func (p *providerManager) GetProviderSchema(ctx context.Context, addr addrs.Provider) ProviderSchema {
	p.schemasLock.Lock()
	schema, ok := p.schemas[addr]
	p.schemasLock.Unlock()
	if ok {
		return schema
	}

	// It's possible that multiple calls in parallel could hit this code, but we can handle that optimization case later
	// For now, we rely on unconfigured being smart enough to produce a single instance for multiple simultaneous calls
	inst, done, err := p.unconfiguredProvider(ctx, addr)
	defer done()

	if err != nil {
		schema = ProviderSchema{Diagnostics: tfdiags.Diagnostics{}.Append(err)}
	} else {
		schema = inst.GetProviderSchema(ctx)

		// This validation originally came from contextPlugins in the tofu (legacy engine) package
		// Depending on how the schema value is cached within the providers.Unconfigured implementation,
		// this could introduce multiple copies of the validated schema errors and should be revisited

		if schema.Provider.Version < 0 {
			// We're not using the version numbers here yet, but we'll check
			// for validity anyway in case we start using them in future.
			schema.Diagnostics = schema.Diagnostics.Append(fmt.Errorf("provider %s has invalid negative schema version for its configuration blocks,which is a bug in the provider ", addr))
		}

		for t, r := range schema.ResourceTypes {
			if err := r.Block.InternalValidate(); err != nil {
				schema.Diagnostics = schema.Diagnostics.Append(fmt.Errorf("provider %s has invalid schema for managed resource type %q, which is a bug in the provider: %w", addr, t, err))
			}
			if r.Version < 0 {
				schema.Diagnostics = schema.Diagnostics.Append(fmt.Errorf("provider %s has invalid negative schema version for managed resource type %q, which is a bug in the provider", addr, t))
			}
		}

		for t, d := range schema.DataSources {
			if err := d.Block.InternalValidate(); err != nil {
				schema.Diagnostics = schema.Diagnostics.Append(fmt.Errorf("provider %s has invalid schema for data resource type %q, which is a bug in the provider: %w", addr, t, err))
			}
			if d.Version < 0 {
				// We're not using the version numbers here yet, but we'll check
				// for validity anyway in case we start using them in future.
				schema.Diagnostics = schema.Diagnostics.Append(fmt.Errorf("provider %s has invalid negative schema version for data resource type %q, which is a bug in the provider", addr, t))
			}
		}

		for t, d := range schema.EphemeralResources {
			if err := d.Block.InternalValidate(); err != nil {
				schema.Diagnostics = schema.Diagnostics.Append(fmt.Errorf("provider %s has invalid schema for ephemeral resource type %q, which is a bug in the provider: %w", addr, t, err))
			}
			if d.Version < 0 {
				// We're not using the version numbers here yet, but we'll check
				// for validity anyway in case we start using them in future.
				schema.Diagnostics = schema.Diagnostics.Append(fmt.Errorf("provider %s has invalid negative schema version for ephemeral resource type %q, which is a bug in the provider", addr, t))
			}
		}

	}

	p.schemasLock.Lock()
	p.schemas[addr] = schema
	p.schemasLock.Unlock()

	return schema
}

func (p *providerManager) ProviderConfigSchema(ctx context.Context, addr addrs.Provider) (*providers.Schema, tfdiags.Diagnostics) {
	schema := p.GetProviderSchema(ctx, addr)
	diags := schema.Diagnostics

	if diags.HasErrors() {
		return nil, diags
	}

	return &schema.Provider, diags
}

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
	return &ret, diags
}

func (p *providerManager) ValidateProviderConfig(ctx context.Context, addr addrs.Provider, cfgVal cty.Value) tfdiags.Diagnostics {
	cfgVal, _ = cfgVal.UnmarkDeep()

	inst, done, err := p.unconfiguredProvider(ctx, addr)
	defer done()
	if err != nil {
		return tfdiags.Diagnostics{}.Append(err)
	}

	// NOTE: we ignore resp.PreparedConfig in this codepath, but not below in ConfigureProvider
	// This is to handle some oddities in tfplugin5, documented in providers.ValidateProviderConfigResponse
	return inst.ValidateProviderConfig(ctx, providers.ValidateProviderConfigRequest{Config: cfgVal}).Diagnostics
}

func (p *providerManager) ValidateResourceConfig(ctx context.Context, addr addrs.Provider, mode addrs.ResourceMode, typeName string, cfgVal cty.Value) tfdiags.Diagnostics {
	cfgVal, _ = cfgVal.UnmarkDeep()

	inst, done, err := p.unconfiguredProvider(ctx, addr)
	defer done()
	if err != nil {
		return tfdiags.Diagnostics{}.Append(err)
	}
	return inst.ValidateResourceConfig(ctx, providers.ValidateResourceConfigRequest{Config: cfgVal}).Diagnostics
}

func (p *providerManager) MoveResourceState(ctx context.Context, addr addrs.Provider, req providers.MoveResourceStateRequest) providers.MoveResourceStateResponse {
	panic("not implemented") // TODO: Implement
}

func (p *providerManager) CallFunction(ctx context.Context, addr addrs.Provider, name string, arguments []cty.Value) (cty.Value, error) {
	panic("not implemented") // TODO: Implement
}

func (p *providerManager) ConfigureProvider(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, cfgVal cty.Value) tfdiags.Diagnostics {
	// TODO consider more granular locking
	p.configuredLock.Lock()
	defer p.configuredLock.Unlock()

	var diags tfdiags.Diagnostics

	key := addr.String()
	instance, ok := p.configured[key]
	if ok {
		return diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Provider already configured",
			fmt.Sprintf("Unable to configure already configured provider at address %q", addr),
		))
	}

	instance, instDiags := p.newProviderInst(ctx, addr.Config.Config.Provider)
	diags = diags.Append(instDiags)
	if diags.HasErrors() {
		return diags
	}

	// Unmark
	cfgVal, _ = cfgVal.UnmarkDeep()

	// Unfortunate interaction with tfplugin5
	validate := instance.ValidateProviderConfig(ctx, providers.ValidateProviderConfigRequest{
		Config: cfgVal,
	})
	diags = diags.Append(validate.Diagnostics)
	if diags.HasErrors() {
		return diags
	}

	// tfplugin5 may return a different PreparedConfig, but we throw that away in all code paths?
	if validate.PreparedConfig != cty.NilVal && !validate.PreparedConfig.IsNull() && !validate.PreparedConfig.RawEquals(cfgVal) {
		log.Printf("[WARN] ValidateProviderConfig from %q changed the config value, but that value is unused", addr)
	}

	configure := instance.ConfigureProvider(ctx, providers.ConfigureProviderRequest{
		Config: cfgVal,

		// We aren't actually Terraform, so we'll just pretend to be a
		// Terraform version that has roughly the same functionality that
		// OpenTofu currently has, since providers are permitted to use this to
		// adapt their behavior for older versions of Terraform.
		TerraformVersion: "1.13.0",
	})
	diags = diags.Append(configure.Diagnostics)
	if diags.HasErrors() {
		return diags
	}

	p.configured[key] = instance

	return diags
}

func (p *providerManager) configuredProvider(addr addrs.AbsProviderInstanceCorrect) providers.Configured {
	p.configuredLock.Lock()
	defer p.configuredLock.Unlock()

	key := addr.String()
	instance, ok := p.configured[key]
	if !ok {
		// TODO should we panic? diagnostics? error?
		panic("BUG")
	}
	return instance
}

func (p *providerManager) UpgradeResourceState(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.UpgradeResourceStateRequest) providers.UpgradeResourceStateResponse {
	configured := p.configuredProvider(addr)
	return configured.UpgradeResourceState(ctx, req)
}

func (p *providerManager) ReadResource(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.ReadResourceRequest) providers.ReadResourceResponse {
	configured := p.configuredProvider(addr)
	return configured.ReadResource(ctx, req)
}

func (p *providerManager) PlanResourceChange(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
	configured := p.configuredProvider(addr)
	return configured.PlanResourceChange(ctx, req)
}

func (p *providerManager) ApplyResourceChange(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
	configured := p.configuredProvider(addr)
	return configured.ApplyResourceChange(ctx, req)
}

func (p *providerManager) ImportResourceState(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.ImportResourceStateRequest) providers.ImportResourceStateResponse {
	configured := p.configuredProvider(addr)
	return configured.ImportResourceState(ctx, req)
}

func (p *providerManager) ReadDataSource(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
	configured := p.configuredProvider(addr)
	return configured.ReadDataSource(ctx, req)
}

func (p *providerManager) OpenEphemeralResource(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.OpenEphemeralResourceRequest) providers.OpenEphemeralResourceResponse {
	configured := p.configuredProvider(addr)
	return configured.OpenEphemeralResource(ctx, req)
}

func (p *providerManager) RenewEphemeralResource(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.RenewEphemeralResourceRequest) providers.RenewEphemeralResourceResponse {
	configured := p.configuredProvider(addr)
	return configured.RenewEphemeralResource(ctx, req)
}

func (p *providerManager) CloseEphemeralResource(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.CloseEphemeralResourceRequest) providers.CloseEphemeralResourceResponse {
	configured := p.configuredProvider(addr)
	return configured.CloseEphemeralResource(ctx, req)
}

func (p *providerManager) GetFunctions(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) providers.GetFunctionsResponse {
	configured := p.configuredProvider(addr)
	return configured.GetFunctions(ctx)
}

func (p *providerManager) CloseProvider(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) error {
	p.configuredLock.Lock()
	defer p.configuredLock.Unlock()

	key := addr.String()
	configured, ok := p.configured[key]
	if !ok {
		return fmt.Errorf("Unable to close provider %s, not configured", key)
	}
	err := configured.Close(ctx)

	// Regardless of if the close operation succeeded, we should remove it from active rotation
	delete(p.configured, key)

	return err
}

func (p *providerManager) Stop(ctx context.Context) error {

	p.configuredLock.Lock()
	defer p.configuredLock.Unlock()

	p.unconfiguredLock.Lock()
	defer p.unconfiguredLock.Unlock()

	var errs []error

	for _, unconfigured := range p.unconfigured {
		errs = append(errs, unconfigured.Stop(ctx))
	}
	for _, configured := range p.configured {
		errs = append(errs, configured.Stop(ctx))
	}

	return errors.Join(errs...)
}
