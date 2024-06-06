// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/hcl2shim"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
)

var _ providers.Interface = providerForTest{}

// providerForTest is a wrapper around real provider to allow certain resources to be overridden
// for testing framework. Currently, it's used in NodeAbstractResourceInstance only in a format
// of one time use. It handles overrideValues and plannedChange for a single resource instance
// (i.e. by certain address).
// TODO: providerForTest should be extended to handle mock providers implementation with per-type
// mocking. It will allow providerForTest to be used for both overrides and full mocking.
// In such scenario, overrideValues should be extended to handle per-type values and plannedChange
// should contain per PlanResourceChangeRequest cache to produce the same plan result
// for the same PlanResourceChangeRequest.
type providerForTest struct {
	// It's not embedded to make it safer to extend providers.Interface
	// without silently breaking providerForTest functionality.
	internal providers.Interface
	schema   providers.ProviderSchema

	overrideValues map[string]cty.Value
	plannedChange  *cty.Value
}

func (p providerForTest) ReadResource(r providers.ReadResourceRequest) providers.ReadResourceResponse {
	var resp providers.ReadResourceResponse

	resSchema, _ := p.schema.SchemaForResourceType(addrs.ManagedResourceMode, r.TypeName)

	resp.NewState, resp.Diagnostics = hcl2shim.ComposeMockValueBySchema(resSchema, r.ProviderMeta, p.overrideValues)
	return resp
}

func (p providerForTest) PlanResourceChange(r providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
	if r.Config.IsNull() {
		return providers.PlanResourceChangeResponse{
			PlannedState: r.ProposedNewState, // null
		}
	}

	if p.plannedChange != nil {
		return providers.PlanResourceChangeResponse{
			PlannedState: *p.plannedChange,
		}
	}

	resSchema, _ := p.schema.SchemaForResourceType(addrs.ManagedResourceMode, r.TypeName)

	var resp providers.PlanResourceChangeResponse

	resp.PlannedState, resp.Diagnostics = hcl2shim.ComposeMockValueBySchema(resSchema, r.Config, p.overrideValues)

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

	resp.State, resp.Diagnostics = hcl2shim.ComposeMockValueBySchema(resSchema, r.Config, p.overrideValues)

	return resp
}

// Calling the internal provider ensures providerForTest has the same behaviour as if
// it wasn't overridden. Some of these functions should be changed in the future to
// support mock_provider (e.g. ConfigureProvider should do nothing), mock_resource and
// mock_data. The only exception is ImportResourceState, which panics if called via providerForTest
// because importing is not supported in testing framework.

func (p providerForTest) GetProviderSchema() providers.GetProviderSchemaResponse {
	return p.internal.GetProviderSchema()
}

func (p providerForTest) ValidateProviderConfig(r providers.ValidateProviderConfigRequest) providers.ValidateProviderConfigResponse {
	return p.internal.ValidateProviderConfig(r)
}

func (p providerForTest) ValidateResourceConfig(r providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse {
	return p.internal.ValidateResourceConfig(r)
}

func (p providerForTest) ValidateDataResourceConfig(r providers.ValidateDataResourceConfigRequest) providers.ValidateDataResourceConfigResponse {
	return p.internal.ValidateDataResourceConfig(r)
}

func (p providerForTest) UpgradeResourceState(r providers.UpgradeResourceStateRequest) providers.UpgradeResourceStateResponse {
	return p.internal.UpgradeResourceState(r)
}

func (p providerForTest) ConfigureProvider(r providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
	return p.internal.ConfigureProvider(r)
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

func (p providerForTest) ImportResourceState(providers.ImportResourceStateRequest) providers.ImportResourceStateResponse {
	panic("Importing is not supported in testing context. providerForTest must not be used to call ImportResourceState")
}
