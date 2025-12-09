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

func NewPluginManager(ctx context.Context,
	providerFactories map[addrs.Provider]providers.Factory,
	provisionerFactories map[string]provisioners.Factory,
) PluginManager {
	return struct {
		ProvisionerManager
		ProviderManager
	}{
		ProvisionerManager: NewProvisionerManager(provisionerFactories),
		ProviderManager:    NewProviderManager(ctx, providerFactories),
	}
}
