package rpcproviders

import (
	"github.com/apparentlymart/opentofu-providers/tofuprovider/providerops"
	"github.com/opentofu/opentofu/internal/providers"
)

var clientCapabilities = &providerops.ClientCapabilities{
	// TODO: Turn this on once we've incorporated similar handling of
	// deferred as we did in the plugin/plugin6 packages.
	SupportsDeferral:            false,
	SupportsWriteOnlyAttributes: false,
}

func convertServerCapabililties(caps providerops.ServerCapabilities) *providers.ServerCapabilities {
	return &providers.ServerCapabilities{
		PlanDestroy:               caps.CanPlanDestroy(),
		GetProviderSchemaOptional: caps.GetProviderSchemaIsOptional(),
		// TODO: What about CanMoveManagedResourceState?
	}
}
