package plugins

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
)

type PluginSchemas interface {
	ProvisionerSchemas
	ProviderSchemas
}

type PluginManager interface {
	ProvisionerManager
	ProviderManager
}

type Plugins interface {
	HasProvider(addr addrs.Provider) bool
	HasProvisioner(typ string) bool

	Schemas(ctx context.Context) PluginSchemas
	Manager(ctx context.Context) PluginManager
}

type plugins struct {
	providerFactories    map[addrs.Provider]providers.Factory
	provisionerFactories map[string]provisioners.Factory
}

func NewPlugins(
	providerFactories map[addrs.Provider]providers.Factory,
	provisionerFactories map[string]provisioners.Factory,
) Plugins {
	return &plugins{
		providerFactories:    providerFactories,
		provisionerFactories: provisionerFactories,
	}
}

func (p *plugins) HasProvider(addr addrs.Provider) bool {
	_, ok := p.providerFactories[addr]
	return ok
}

func (p *plugins) HasProvisioner(typ string) bool {
	_, ok := p.provisionerFactories[typ]
	return ok
}

func (p *plugins) Schemas(ctx context.Context) PluginSchemas {
	return struct {
		ProvisionerSchemas
		ProviderSchemas
	}{
		ProvisionerSchemas: NewProvisionerManager(p.provisionerFactories),
		ProviderSchemas:    NewProviderManager(ctx, p.providerFactories),
	}
}

func (p *plugins) Manager(ctx context.Context) PluginManager {
	return struct {
		ProvisionerManager
		ProviderManager
	}{
		ProvisionerManager: NewProvisionerManager(p.provisionerFactories),
		ProviderManager:    NewProviderManager(ctx, p.providerFactories),
	}
}
