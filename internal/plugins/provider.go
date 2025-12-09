package plugins

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

type ProviderSchema = providers.ProviderSchema

type ProviderSchemas interface {
	HasProvider(addr addrs.Provider) bool

	// GetMetadata is not yet implemented or used at this time
	GetProviderSchema(ctx context.Context, addr addrs.Provider) ProviderSchema

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
}

type ProviderManager interface {
	ProviderSchemas

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

	ConfigureProvider(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, cfgVal cty.Value) tfdiags.Diagnostics

	IsProviderConfigured(addr addrs.AbsProviderInstanceCorrect) bool
	ConfiguredProvider(addr addrs.AbsProviderInstanceCorrect) providers.Configured

	UpgradeResourceState(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.UpgradeResourceStateRequest) providers.UpgradeResourceStateResponse
	ReadResource(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.ReadResourceRequest) providers.ReadResourceResponse
	PlanResourceChange(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse
	ApplyResourceChange(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse
	ImportResourceState(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.ImportResourceStateRequest) providers.ImportResourceStateResponse
	ReadDataSource(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse
	OpenEphemeralResource(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.OpenEphemeralResourceRequest) providers.OpenEphemeralResourceResponse
	RenewEphemeralResource(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.RenewEphemeralResourceRequest) providers.RenewEphemeralResourceResponse
	CloseEphemeralResource(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, req providers.CloseEphemeralResourceRequest) providers.CloseEphemeralResourceResponse

	// These are weird due to
	GetFunctions(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) providers.GetFunctionsResponse
	CallFunction(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, name string, arguments []cty.Value) (cty.Value, error)

	CloseProvider(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) error

	Stop(ctx context.Context) error
}
