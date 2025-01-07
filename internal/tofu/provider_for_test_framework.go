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

	mockResources     mockResourcesForTest
	overrideResources overrideResourcesForTest

	currentResourceAddress string
}

func newProviderForTestWithSchema(internal providers.Interface, schema providers.ProviderSchema) (providerForTest, error) {
	if p, ok := internal.(providerForTest); ok {
		// We can create a proper deep copy here, however currently
		// it is only relevant for override resources, since we extend
		// the override resource map in NodeAbstractResourceInstance.
		return p.withCopiedOverrideResources(), nil
	}

	if schema.Diagnostics.HasErrors() {
		return providerForTest{}, fmt.Errorf("invalid provider schema for test wrapper: %w", schema.Diagnostics.Err())
	}

	return providerForTest{
		internal: internal,
		schema:   schema,
		mockResources: mockResourcesForTest{
			managed: make(map[string]resourceForTest),
			data:    make(map[string]resourceForTest),
		},
		overrideResources: overrideResourcesForTest{
			managed: make(map[string]resourceForTest),
			data:    make(map[string]resourceForTest),
		},
	}, nil
}

func (p providerForTest) ReadResource(r providers.ReadResourceRequest) providers.ReadResourceResponse {
	resSchema, _ := p.schema.SchemaForResourceType(addrs.ManagedResourceMode, r.TypeName)

	mockValues := p.getMockValuesForManagedResource(r.TypeName)

	var resp providers.ReadResourceResponse

	resp.NewState, resp.Diagnostics = newMockValueComposer(r.TypeName).
		ComposeBySchema(resSchema, r.ProviderMeta, mockValues)

	return resp
}

func (p providerForTest) PlanResourceChange(r providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
	if r.Config.IsNull() {
		return providers.PlanResourceChangeResponse{
			PlannedState: r.ProposedNewState, // null
		}
	}

	resSchema, _ := p.schema.SchemaForResourceType(addrs.ManagedResourceMode, r.TypeName)

	mockValues := p.getMockValuesForManagedResource(r.TypeName)

	var resp providers.PlanResourceChangeResponse

	resp.PlannedState, resp.Diagnostics = newMockValueComposer(r.TypeName).
		ComposeBySchema(resSchema, r.Config, mockValues)

	return resp
}

func (p providerForTest) ApplyResourceChange(r providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
	return providers.ApplyResourceChangeResponse{
		NewState: r.PlannedState,
	}
}

func (p providerForTest) ReadDataSource(r providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
	resSchema, _ := p.schema.SchemaForResourceType(addrs.DataResourceMode, r.TypeName)

	var resp providers.ReadDataSourceResponse

	mockValues := p.getMockValuesForDataResource(r.TypeName)

	resp.State, resp.Diagnostics = newMockValueComposer(r.TypeName).
		ComposeBySchema(resSchema, r.Config, mockValues)

	return resp
}

// ValidateProviderConfig is irrelevant when provider is mocked or overridden.
func (p providerForTest) ValidateProviderConfig(_ providers.ValidateProviderConfigRequest) providers.ValidateProviderConfigResponse {
	return providers.ValidateProviderConfigResponse{}
}

// GetProviderSchema is also used to perform additional validation outside of the provider
// implementation. We are excluding parts of the schema related to provider since it is
// irrelevant in the scope of mocking / overriding. When running `tofu test` configuration
// is being transformed for testing framework and original provider configuration is not
// accessible so it is safe to wipe metadata as well. See Config.transformProviderConfigsForTest
// for more details.
func (p providerForTest) GetProviderSchema() providers.GetProviderSchemaResponse {
	providerSchema := p.internal.GetProviderSchema()
	providerSchema.Provider = providers.Schema{}
	providerSchema.ProviderMeta = providers.Schema{}
	return providerSchema
}

// providerForTest doesn't configure its internal provider because it is mocked.
func (p providerForTest) ConfigureProvider(_ providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
	return providers.ConfigureProviderResponse{}
}

func (p providerForTest) ImportResourceState(providers.ImportResourceStateRequest) providers.ImportResourceStateResponse {
	panic("Importing is not supported in testing context. providerForTest must not be used to call ImportResourceState")
}

// Calling the internal provider ensures providerForTest has the same behaviour as if
// it wasn't overridden or mocked. The only exception is ImportResourceState, which panics
// if called via providerForTest because importing is not supported in testing framework.

func (p providerForTest) ValidateResourceConfig(r providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse {
	return p.internal.ValidateResourceConfig(r)
}

func (p providerForTest) ValidateDataResourceConfig(r providers.ValidateDataResourceConfigRequest) providers.ValidateDataResourceConfigResponse {
	return p.internal.ValidateDataResourceConfig(r)
}

func (p providerForTest) UpgradeResourceState(r providers.UpgradeResourceStateRequest) providers.UpgradeResourceStateResponse {
	return p.internal.UpgradeResourceState(r)
}

func (p providerForTest) Stop() error {
	return p.internal.Stop()
}

func (p providerForTest) GetFunctions() providers.GetFunctionsResponse {
	return p.internal.GetFunctions()
}

func (p providerForTest) CallFunction(r providers.CallFunctionRequest) providers.CallFunctionResponse {
	return p.internal.CallFunction(r)
}

func (p providerForTest) Close() error {
	return p.internal.Close()
}

func (p providerForTest) withMockResources(mockResources []*configs.MockResource) providerForTest {
	for _, res := range mockResources {
		var resources map[mockResourceType]resourceForTest

		switch res.Mode {
		case addrs.ManagedResourceMode:
			resources = p.mockResources.managed
		case addrs.DataResourceMode:
			resources = p.mockResources.data
		case addrs.InvalidResourceMode:
			panic("BUG: invalid mock resource mode")
		default:
			panic("BUG: unsupported mock resource mode: " + res.Mode.String())
		}

		resources[res.Type] = resourceForTest{
			values: res.Defaults,
		}
	}

	return p
}

func (p providerForTest) withCopiedOverrideResources() providerForTest {
	p.overrideResources = p.overrideResources.copy()
	return p
}

func (p providerForTest) withOverrideResources(overrideResources []*configs.OverrideResource) providerForTest {
	for _, res := range overrideResources {
		p = p.withOverrideResource(*res.TargetParsed, res.Values)
	}

	return p
}

func (p providerForTest) withOverrideResource(addr addrs.ConfigResource, overrides map[string]cty.Value) providerForTest {
	var resources map[string]resourceForTest

	switch addr.Resource.Mode {
	case addrs.ManagedResourceMode:
		resources = p.overrideResources.managed
	case addrs.DataResourceMode:
		resources = p.overrideResources.data
	case addrs.InvalidResourceMode:
		panic("BUG: invalid override resource mode")
	default:
		panic("BUG: unsupported override resource mode: " + addr.Resource.Mode.String())
	}

	resources[addr.String()] = resourceForTest{
		values: overrides,
	}

	return p
}

func (p providerForTest) linkWithCurrentResource(addr addrs.ConfigResource) providerForTest {
	p.currentResourceAddress = addr.String()
	return p
}

type resourceForTest struct {
	values map[string]cty.Value
}

type mockResourceType = string

type mockResourcesForTest struct {
	managed map[mockResourceType]resourceForTest
	data    map[mockResourceType]resourceForTest
}

type overrideResourceAddress = string

type overrideResourcesForTest struct {
	managed map[overrideResourceAddress]resourceForTest
	data    map[overrideResourceAddress]resourceForTest
}

func (res overrideResourcesForTest) copy() overrideResourcesForTest {
	resCopy := overrideResourcesForTest{
		managed: make(map[overrideResourceAddress]resourceForTest, len(res.managed)),
		data:    make(map[overrideResourceAddress]resourceForTest, len(res.data)),
	}

	for k, v := range res.managed {
		resCopy.managed[k] = v
	}

	for k, v := range res.data {
		resCopy.data[k] = v
	}

	return resCopy
}

func (p providerForTest) getMockValuesForManagedResource(typeName string) map[string]cty.Value {
	if p.currentResourceAddress != "" {
		res, ok := p.overrideResources.managed[p.currentResourceAddress]
		if ok {
			return res.values
		}
	}

	return p.mockResources.managed[typeName].values
}

func (p providerForTest) getMockValuesForDataResource(typeName string) map[string]cty.Value {
	if p.currentResourceAddress != "" {
		res, ok := p.overrideResources.data[p.currentResourceAddress]
		if ok {
			return res.values
		}
	}

	return p.mockResources.data[typeName].values
}

func newMockValueComposer(typeName string) hcl2shim.MockValueComposer {
	hash := fnv.New32()
	hash.Write([]byte(typeName))
	return hcl2shim.NewMockValueComposer(int64(hash.Sum32()))
}
