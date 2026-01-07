// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/configs/hcl2shim"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

var _ providers.Interface = &providerForTest{}

// providerForTest is a wrapper around a real provider to allow a specific resource to be overridden
// for the testing framework.  It is used by [NodeResourceAbstractInstance.getProvider] to fulfil the mocks
// and overrides for that one specific resource instance, any other usage is a bug in OpenTofu and should
// be corrected.
type providerForTest struct {
	// providers.Interface is not embedded to make it safer to extend
	// the interface without silently breaking providerForTest functionality.
	internal providers.Interface
	schema   providers.ProviderSchema

	overrideValues map[string]cty.Value
}

func newProviderForTestWithSchema(internal providers.Interface, schema providers.ProviderSchema, overrideValues map[string]cty.Value) (providerForTest, error) {
	if schema.Diagnostics.HasErrors() {
		return providerForTest{}, fmt.Errorf("invalid provider schema for test wrapper: %w", schema.Diagnostics.Err())
	}

	return providerForTest{
		internal:       internal,
		schema:         schema,
		overrideValues: overrideValues,
	}, nil
}

func (p providerForTest) ReadResource(_ context.Context, r providers.ReadResourceRequest) providers.ReadResourceResponse {
	var resp providers.ReadResourceResponse

	resp.NewState = r.PriorState
	if resp.NewState.IsNull() {
		resp.Diagnostics = tfdiags.Diagnostics{}.Append(tfdiags.WholeContainingBody(
			tfdiags.Error,
			fmt.Sprintf("Unexpected null value for prior state in `%v`", r.TypeName),
			"While reading a resource from a mock provider, the prior state was found to be missing.",
		))
	}

	return resp
}

func (p providerForTest) PlanResourceChange(_ context.Context, r providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
	if r.Config.IsNull() {
		return providers.PlanResourceChangeResponse{
			PlannedState: r.ProposedNewState, // null
		}
	}

	resSchema, _ := p.schema.SchemaForResourceType(addrs.ManagedResourceMode, r.TypeName)

	// Filter out computed-only attributes from the schema to avoid them being used incorrectly
	// later on. This resolves https://github.com/opentofu/opentofu/issues/3644
	filteredConfig := filterComputedOnlyAttributes(resSchema, r.Config)

	var resp providers.PlanResourceChangeResponse
	resp.PlannedState, resp.Diagnostics = newMockValueComposer(r.TypeName).
		ComposeBySchema(resSchema, filteredConfig, p.overrideValues)

	return resp
}

// filterComputedOnlyAttributes returns a copy of value where all computed-only attributes
// (i.e. computed and not optional) defined in resSchema are replaced with null values.
func filterComputedOnlyAttributes(resSchema *configschema.Block, value cty.Value) cty.Value {
	if resSchema == nil || value.IsNull() || !value.IsKnown() {
		// Nothing to filter here, or we dont know how to filter
		// so we should move on
		return value
	}

	ret, _ := cty.Transform(value, func(path cty.Path, v cty.Value) (cty.Value, error) {
		attr := resSchema.AttributeByPath(path)
		if attr != nil && attr.Computed && !attr.Optional {
			return cty.NullVal(v.Type()), nil
		}
		return v, nil
	})
	return ret
}

func (p providerForTest) ApplyResourceChange(_ context.Context, r providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
	return providers.ApplyResourceChangeResponse{
		NewState: r.PlannedState,
	}
}

func (p providerForTest) ReadDataSource(_ context.Context, r providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
	resSchema, _ := p.schema.SchemaForResourceType(addrs.DataResourceMode, r.TypeName)

	var resp providers.ReadDataSourceResponse

	resp.State, resp.Diagnostics = newMockValueComposer(r.TypeName).
		ComposeBySchema(resSchema, r.Config, p.overrideValues)

	return resp
}

func (p providerForTest) OpenEphemeralResource(_ context.Context, _ providers.OpenEphemeralResourceRequest) (resp providers.OpenEphemeralResourceResponse) {
	// TODO ephemeral testing support - implement me when adding testing support
	panic("implement me")
}

func (p providerForTest) RenewEphemeralResource(_ context.Context, _ providers.RenewEphemeralResourceRequest) (resp providers.RenewEphemeralResourceResponse) {
	// TODO ephemeral testing support - implement me when adding testing support
	panic("implement me")
}

func (p providerForTest) CloseEphemeralResource(_ context.Context, _ providers.CloseEphemeralResourceRequest) (resp providers.CloseEphemeralResourceResponse) {
	// TODO ephemeral testing support - implement me when adding testing support
	panic("implement me")
}

// ValidateProviderConfig is irrelevant when provider is mocked or overridden.
func (p providerForTest) ValidateProviderConfig(_ context.Context, _ providers.ValidateProviderConfigRequest) providers.ValidateProviderConfigResponse {
	return providers.ValidateProviderConfigResponse{}
}

// GetProviderSchema is also used to perform additional validation outside of the provider
// implementation. We are excluding parts of the schema related to provider since it is
// irrelevant in the scope of mocking / overriding. When running `tofu test` configuration
// is being transformed for testing framework and original provider configuration is not
// accessible so it is safe to wipe metadata as well. See Config.transformProviderConfigsForTest
// for more details.
func (p providerForTest) GetProviderSchema(ctx context.Context) providers.GetProviderSchemaResponse {
	providerSchema := p.internal.GetProviderSchema(ctx)
	providerSchema.Provider = providers.Schema{}
	providerSchema.ProviderMeta = providers.Schema{}
	return providerSchema
}

// providerForTest doesn't configure its internal provider because it is mocked.
func (p providerForTest) ConfigureProvider(context.Context, providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
	return providers.ConfigureProviderResponse{}
}

func (p providerForTest) ImportResourceState(context.Context, providers.ImportResourceStateRequest) providers.ImportResourceStateResponse {
	panic("Importing is not supported in testing context. providerForTest must not be used to call ImportResourceState")
}

func (p providerForTest) MoveResourceState(context.Context, providers.MoveResourceStateRequest) providers.MoveResourceStateResponse {
	panic("Moving is not supported in testing context. providerForTest must not be used to call MoveResourceState")
}

// Calling the internal provider ensures providerForTest has the same behaviour as if
// it wasn't overridden or mocked. The only exception is ImportResourceState, which panics
// if called via providerForTest because importing is not supported in testing framework.

func (p providerForTest) ValidateResourceConfig(ctx context.Context, r providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse {
	return p.internal.ValidateResourceConfig(ctx, r)
}

func (p providerForTest) ValidateDataResourceConfig(ctx context.Context, r providers.ValidateDataResourceConfigRequest) providers.ValidateDataResourceConfigResponse {
	return p.internal.ValidateDataResourceConfig(ctx, r)
}

func (p providerForTest) UpgradeResourceState(ctx context.Context, r providers.UpgradeResourceStateRequest) providers.UpgradeResourceStateResponse {
	return p.internal.UpgradeResourceState(ctx, r)
}

func (p providerForTest) ValidateEphemeralConfig(ctx context.Context, request providers.ValidateEphemeralConfigRequest) providers.ValidateEphemeralConfigResponse {
	return p.internal.ValidateEphemeralConfig(ctx, request)
}

func (p providerForTest) Stop(ctx context.Context) error {
	return p.internal.Stop(ctx)
}

func (p providerForTest) GetFunctions(ctx context.Context) providers.GetFunctionsResponse {
	return p.internal.GetFunctions(ctx)
}

func (p providerForTest) CallFunction(ctx context.Context, r providers.CallFunctionRequest) providers.CallFunctionResponse {
	return p.internal.CallFunction(ctx, r)
}

func (p providerForTest) Close(ctx context.Context) error {
	return p.internal.Close(ctx)
}

func newMockValueComposer(typeName string) hcl2shim.MockValueComposer {
	hash := fnv.New32()
	hash.Write([]byte(typeName))
	return hcl2shim.NewMockValueComposer(int64(hash.Sum32()))
}
