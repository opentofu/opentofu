// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"context"
	"errors"
	"sync"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type MockExecContext struct {
	Calls []MockExecContextCall

	DesiredResourceInstanceFunc    func(ctx context.Context, addr addrs.AbsResourceInstance) *eval.DesiredResourceInstance
	NewProviderClientFunc          func(ctx context.Context, addr addrs.Provider, configVal cty.Value) (providers.Configured, tfdiags.Diagnostics)
	ProviderInstanceConfigFunc     func(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) cty.Value
	ResourceInstancePriorStateFunc func(ctx context.Context, addr addrs.AbsResourceInstance, deposedKey states.DeposedKey) *states.ResourceInstanceObject

	mu sync.Mutex
}

// DesiredResourceInstance implements ExecContext.
func (m *MockExecContext) DesiredResourceInstance(ctx context.Context, addr addrs.AbsResourceInstance) *eval.DesiredResourceInstance {
	var result *eval.DesiredResourceInstance
	if m.DesiredResourceInstanceFunc != nil {
		result = m.DesiredResourceInstanceFunc(ctx, addr)
	}
	m.appendLog("DesiredResourceInstance", []any{addr}, result)
	return result
}

// NewProviderClient implements ExecContext.
func (m *MockExecContext) NewProviderClient(ctx context.Context, addr addrs.Provider, configVal cty.Value) (providers.Configured, tfdiags.Diagnostics) {
	var result providers.Configured
	var diags tfdiags.Diagnostics
	if m.NewProviderClientFunc != nil {
		result, diags = m.NewProviderClientFunc(ctx, addr, configVal)
	} else {
		diags = diags.Append(errors.New("no provider clients available in this MockExecContext"))
	}
	m.appendLog("NewProviderClient", []any{addr, configVal}, result)
	return result, diags
}

// ProviderInstanceConfig implements ExecContext.
func (m *MockExecContext) ProviderInstanceConfig(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) cty.Value {
	var result cty.Value
	if m.ProviderInstanceConfigFunc != nil {
		result = m.ProviderInstanceConfigFunc(ctx, addr)
	}
	m.appendLog("ProviderInstanceConfig", []any{addr}, result)
	return result
}

// ResourceInstancePriorState implements ExecContext.
func (m *MockExecContext) ResourceInstancePriorState(ctx context.Context, addr addrs.AbsResourceInstance, deposedKey states.DeposedKey) *states.ResourceInstanceObject {
	var result *states.ResourceInstanceObject
	if m.ResourceInstancePriorStateFunc != nil {
		result = m.ResourceInstancePriorStateFunc(ctx, addr, deposedKey)
	}
	m.appendLog("ResourceInstancePriorState", []any{addr, deposedKey}, result)
	return result
}

func (m *MockExecContext) NewManagedResourceProviderClient(
	planFunc func(context.Context, providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse,
	applyFunc func(context.Context, providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse,
) providers.Configured {
	return &managedResourceInstanceMockProvider{
		PlanResourceChangeFunc:  planFunc,
		ApplyResourceChangeFunc: applyFunc,
		execCtx:                 m,
	}
}

func (m *MockExecContext) appendLog(methodName string, args []any, result any) {
	//log.Printf("[TRACE] execgraph.MockExecContext: %s(%#v) -> %#v", methodName, args, result)
	m.mu.Lock()
	m.Calls = append(m.Calls, MockExecContextCall{
		MethodName: methodName,
		Args:       args,
		Result:     result,
	})
	m.mu.Unlock()
}

var _ ExecContext = (*MockExecContext)(nil)

type MockExecContextCall struct {
	MethodName string
	Args       []any
	Result     any
}

type managedResourceInstanceMockProvider struct {
	PlanResourceChangeFunc  func(ctx context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse
	ApplyResourceChangeFunc func(ctx context.Context, req providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse
	execCtx                 *MockExecContext
}

var _ providers.Configured = (*managedResourceInstanceMockProvider)(nil)

// ApplyResourceChange implements providers.Configured.
func (m *managedResourceInstanceMockProvider) ApplyResourceChange(ctx context.Context, req providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
	resp := m.ApplyResourceChangeFunc(ctx, req)
	m.execCtx.appendLog("providerClient.ApplyResourceChange", []any{req}, resp)
	return resp
}

// CallFunction implements providers.Configured.
func (m *managedResourceInstanceMockProvider) CallFunction(context.Context, providers.CallFunctionRequest) providers.CallFunctionResponse {
	panic("unimplemented")
}

// Close implements providers.Configured.
func (m *managedResourceInstanceMockProvider) Close(context.Context) error {
	return nil
}

// CloseEphemeralResource implements providers.Configured.
func (m *managedResourceInstanceMockProvider) CloseEphemeralResource(context.Context, providers.CloseEphemeralResourceRequest) providers.CloseEphemeralResourceResponse {
	panic("unimplemented")
}

// ConfigureProvider implements providers.Configured.
func (m *managedResourceInstanceMockProvider) ConfigureProvider(context.Context, providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
	return providers.ConfigureProviderResponse{}
}

// GetFunctions implements providers.Configured.
func (m *managedResourceInstanceMockProvider) GetFunctions(context.Context) providers.GetFunctionsResponse {
	panic("unimplemented")
}

// GetProviderSchema implements providers.Configured.
func (m *managedResourceInstanceMockProvider) GetProviderSchema(context.Context) providers.GetProviderSchemaResponse {
	panic("unimplemented")
}

// ImportResourceState implements providers.Configured.
func (m *managedResourceInstanceMockProvider) ImportResourceState(context.Context, providers.ImportResourceStateRequest) providers.ImportResourceStateResponse {
	panic("unimplemented")
}

// MoveResourceState implements providers.Configured.
func (m *managedResourceInstanceMockProvider) MoveResourceState(context.Context, providers.MoveResourceStateRequest) providers.MoveResourceStateResponse {
	panic("unimplemented")
}

// OpenEphemeralResource implements providers.Configured.
func (m *managedResourceInstanceMockProvider) OpenEphemeralResource(context.Context, providers.OpenEphemeralResourceRequest) providers.OpenEphemeralResourceResponse {
	panic("unimplemented")
}

// PlanResourceChange implements providers.Configured.
func (m *managedResourceInstanceMockProvider) PlanResourceChange(ctx context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
	resp := m.PlanResourceChangeFunc(ctx, req)
	m.execCtx.appendLog("providerClient.PlanResourceChange", []any{req}, resp)
	return resp
}

// ReadDataSource implements providers.Configured.
func (m *managedResourceInstanceMockProvider) ReadDataSource(context.Context, providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
	panic("unimplemented")
}

// ReadResource implements providers.Configured.
func (m *managedResourceInstanceMockProvider) ReadResource(context.Context, providers.ReadResourceRequest) providers.ReadResourceResponse {
	panic("unimplemented")
}

// RenewEphemeralResource implements providers.Configured.
func (m *managedResourceInstanceMockProvider) RenewEphemeralResource(context.Context, providers.RenewEphemeralResourceRequest) (resp providers.RenewEphemeralResourceResponse) {
	panic("unimplemented")
}

// Stop implements providers.Configured.
func (m *managedResourceInstanceMockProvider) Stop(context.Context) error {
	panic("unimplemented")
}

// UpgradeResourceState implements providers.Configured.
func (m *managedResourceInstanceMockProvider) UpgradeResourceState(context.Context, providers.UpgradeResourceStateRequest) providers.UpgradeResourceStateResponse {
	panic("unimplemented")
}

// ValidateDataResourceConfig implements providers.Configured.
func (m *managedResourceInstanceMockProvider) ValidateDataResourceConfig(context.Context, providers.ValidateDataResourceConfigRequest) providers.ValidateDataResourceConfigResponse {
	panic("unimplemented")
}

// ValidateEphemeralConfig implements providers.Configured.
func (m *managedResourceInstanceMockProvider) ValidateEphemeralConfig(context.Context, providers.ValidateEphemeralConfigRequest) providers.ValidateEphemeralConfigResponse {
	panic("unimplemented")
}

// ValidateProviderConfig implements providers.Configured.
func (m *managedResourceInstanceMockProvider) ValidateProviderConfig(context.Context, providers.ValidateProviderConfigRequest) providers.ValidateProviderConfigResponse {
	panic("unimplemented")
}

// ValidateResourceConfig implements providers.Configured.
func (m *managedResourceInstanceMockProvider) ValidateResourceConfig(context.Context, providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse {
	panic("unimplemented")
}
