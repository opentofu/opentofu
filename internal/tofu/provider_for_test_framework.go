// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"hash/fnv"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/hcl2shim"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
)

var _ providers.Interface = &providerForTest{}

// providerForTest is a wrapper around a real provider to allow certain resources to be overridden
// (by address) or mocked (by provider and resource type) for testing framework.
type providerForTest struct {
	// providers.Interface is not embedded to make it safer to extend
	// the interface without silently breaking providerForTest functionality.
	internal providers.Interface
	schema   providers.ProviderSchema

	managedResources resourceForTestByType
	dataResources    resourceForTestByType
}

func newProviderForTestWithSchema(internal providers.Interface, schema providers.ProviderSchema) *providerForTest {
	return &providerForTest{
		internal:         internal,
		schema:           schema,
		managedResources: make(resourceForTestByType),
		dataResources:    make(resourceForTestByType),
	}
}

func newProviderForTest(internal providers.Interface, res []*configs.MockResource) (providers.Interface, error) {
	schema := internal.GetProviderSchema()
	if schema.Diagnostics.HasErrors() {
		return nil, fmt.Errorf("getting provider schema for test wrapper: %w", schema.Diagnostics.Err())
	}

	p := newProviderForTestWithSchema(internal, schema)

	p.addMockResources(res)

	return p, nil
}

func (p *providerForTest) ReadResource(r providers.ReadResourceRequest) providers.ReadResourceResponse {
	var resp providers.ReadResourceResponse

	resSchema, _ := p.schema.SchemaForResourceType(addrs.ManagedResourceMode, r.TypeName)

	overrideValues := p.managedResources.getOverrideValues(r.TypeName)

	resp.NewState, resp.Diagnostics = newMockValueComposer(r.TypeName).
		ComposeBySchema(resSchema, r.ProviderMeta, overrideValues)

	return resp
}

func (p *providerForTest) PlanResourceChange(r providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
	if r.Config.IsNull() {
		return providers.PlanResourceChangeResponse{
			PlannedState: r.ProposedNewState, // null
		}
	}

	resSchema, _ := p.schema.SchemaForResourceType(addrs.ManagedResourceMode, r.TypeName)

	overrideValues := p.managedResources.getOverrideValues(r.TypeName)

	var resp providers.PlanResourceChangeResponse

	resp.PlannedState, resp.Diagnostics = newMockValueComposer(r.TypeName).
		ComposeBySchema(resSchema, r.Config, overrideValues)

	return resp
}

func (p *providerForTest) ApplyResourceChange(r providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
	return providers.ApplyResourceChangeResponse{
		NewState: r.PlannedState,
	}
}

func (p *providerForTest) ReadDataSource(r providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
	resSchema, _ := p.schema.SchemaForResourceType(addrs.DataResourceMode, r.TypeName)

	var resp providers.ReadDataSourceResponse

	overrideValues := p.dataResources.getOverrideValues(r.TypeName)

	resp.State, resp.Diagnostics = newMockValueComposer(r.TypeName).
		ComposeBySchema(resSchema, r.Config, overrideValues)

	return resp
}

// ValidateProviderConfig is irrelevant when provider is mocked or overridden.
func (p *providerForTest) ValidateProviderConfig(_ providers.ValidateProviderConfigRequest) providers.ValidateProviderConfigResponse {
	return providers.ValidateProviderConfigResponse{}
}

// GetProviderSchema is also used to perform additional validation outside of the provider
// implementation. We are excluding parts of the schema related to provider since it is
// irrelevant in the scope of mocking / overriding. When running `tofu test` configuration
// is being transformed for testing framework and original provider configuration is not
// accessible so it is safe to wipe metadata as well. See Config.transformProviderConfigsForTest
// for more details.
func (p *providerForTest) GetProviderSchema() providers.GetProviderSchemaResponse {
	providerSchema := p.internal.GetProviderSchema()
	providerSchema.Provider = providers.Schema{}
	providerSchema.ProviderMeta = providers.Schema{}
	return providerSchema
}

// providerForTest doesn't configure its internal provider because it is mocked.
func (p *providerForTest) ConfigureProvider(_ providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
	return providers.ConfigureProviderResponse{}
}

func (p *providerForTest) ImportResourceState(providers.ImportResourceStateRequest) providers.ImportResourceStateResponse {
	panic("Importing is not supported in testing context. providerForTest must not be used to call ImportResourceState")
}

func (p *providerForTest) setSingleResource(addr addrs.Resource, overrides map[string]cty.Value) {
	res := resourceForTest{
		overrideValues: overrides,
	}

	switch addr.Mode {
	case addrs.ManagedResourceMode:
		p.managedResources[addr.Type] = res
	case addrs.DataResourceMode:
		p.dataResources[addr.Type] = res
	case addrs.InvalidResourceMode:
		panic("BUG: invalid mock resource mode")
	default:
		panic("BUG: unsupported resource mode: " + addr.Mode.String())
	}
}

func (p *providerForTest) addMockResources(mockResources []*configs.MockResource) {
	for _, mockRes := range mockResources {
		var resources resourceForTestByType

		switch mockRes.Mode {
		case addrs.ManagedResourceMode:
			resources = p.managedResources
		case addrs.DataResourceMode:
			resources = p.dataResources
		case addrs.InvalidResourceMode:
			panic("BUG: invalid mock resource mode")
		default:
			panic("BUG: unsupported mock resource mode: " + mockRes.Mode.String())
		}

		resources[mockRes.Type] = resourceForTest{
			overrideValues: mockRes.Defaults,
		}
	}
}

// Calling the internal provider ensures providerForTest has the same behaviour as if
// it wasn't overridden or mocked. The only exception is ImportResourceState, which panics
// if called via providerForTest because importing is not supported in testing framework.

func (p *providerForTest) ValidateResourceConfig(r providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse {
	return p.internal.ValidateResourceConfig(r)
}

func (p *providerForTest) ValidateDataResourceConfig(r providers.ValidateDataResourceConfigRequest) providers.ValidateDataResourceConfigResponse {
	return p.internal.ValidateDataResourceConfig(r)
}

func (p *providerForTest) UpgradeResourceState(r providers.UpgradeResourceStateRequest) providers.UpgradeResourceStateResponse {
	return p.internal.UpgradeResourceState(r)
}

func (p *providerForTest) Stop() error {
	return p.internal.Stop()
}

func (p *providerForTest) GetFunctions() providers.GetFunctionsResponse {
	return p.internal.GetFunctions()
}

func (p *providerForTest) CallFunction(r providers.CallFunctionRequest) providers.CallFunctionResponse {
	return p.internal.CallFunction(r)
}

func (p *providerForTest) Close() error {
	return p.internal.Close()
}

type resourceForTest struct {
	overrideValues map[string]cty.Value
}

type resourceForTestByType map[string]resourceForTest

func (m resourceForTestByType) getOverrideValues(typeName string) map[string]cty.Value {
	return m[typeName].overrideValues
}

func newMockValueComposer(typeName string) hcl2shim.MockValueComposer {
	hash := fnv.New32()
	hash.Write([]byte(typeName))
	return hcl2shim.NewMockValueComposer(int64(hash.Sum32()))
}
