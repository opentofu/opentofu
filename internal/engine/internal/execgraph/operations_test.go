// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"context"
	"sync"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type mockOperations struct {
	Calls []mockOperationsCall

	DataReadFunc                       func(ctx context.Context, desired *eval.DesiredResourceInstance, plannedVal cty.Value, providerClient *exec.ProviderClient) (*exec.ResourceInstanceObject, tfdiags.Diagnostics)
	EphemeralCloseFunc                 func(ctx context.Context, ephemeral *exec.OpenEphemeralResourceInstance) tfdiags.Diagnostics
	EphemeralOpenFunc                  func(ctx context.Context, desired *eval.DesiredResourceInstance, providerClient *exec.ProviderClient) (*exec.OpenEphemeralResourceInstance, tfdiags.Diagnostics)
	EphemeralStateFunc                 func(ctx context.Context, ephemeral *exec.OpenEphemeralResourceInstance) (*exec.ResourceInstanceObject, tfdiags.Diagnostics)
	ManagedAlreadyDeposedFunc          func(ctx context.Context, instAddr addrs.AbsResourceInstance, deposedKey states.DeposedKey) (*exec.ResourceInstanceObject, tfdiags.Diagnostics)
	ManagedApplyFunc                   func(ctx context.Context, plan *exec.ManagedResourceObjectFinalPlan, fallback *exec.ResourceInstanceObject, providerClient *exec.ProviderClient) (*exec.ResourceInstanceObject, tfdiags.Diagnostics)
	ManagedChangeAddrFunc              func(ctx context.Context, currentObj *exec.ResourceInstanceObject, newAddr addrs.AbsResourceInstance) (*exec.ResourceInstanceObject, tfdiags.Diagnostics)
	ManagedDeposeFunc                  func(ctx context.Context, currentObj *exec.ResourceInstanceObject) (*exec.ResourceInstanceObject, tfdiags.Diagnostics)
	ManagedFinalPlanFunc               func(ctx context.Context, desired *eval.DesiredResourceInstance, prior *exec.ResourceInstanceObject, plannedVal cty.Value, providerClient *exec.ProviderClient) (*exec.ManagedResourceObjectFinalPlan, tfdiags.Diagnostics)
	ProviderInstanceCloseFunc          func(ctx context.Context, client *exec.ProviderClient) tfdiags.Diagnostics
	ProviderInstanceConfigFunc         func(ctx context.Context, instAddr addrs.AbsProviderInstanceCorrect) (*exec.ProviderInstanceConfig, tfdiags.Diagnostics)
	ProviderInstanceOpenFunc           func(ctx context.Context, config *exec.ProviderInstanceConfig) (*exec.ProviderClient, tfdiags.Diagnostics)
	ResourceInstanceDesiredFunc        func(ctx context.Context, instAddr addrs.AbsResourceInstance) (*eval.DesiredResourceInstance, tfdiags.Diagnostics)
	ResourceInstancePostconditionsFunc func(ctx context.Context, result *exec.ResourceInstanceObject) tfdiags.Diagnostics
	ResourceInstancePriorFunc          func(ctx context.Context, instAddr addrs.AbsResourceInstance) (*exec.ResourceInstanceObject, tfdiags.Diagnostics)

	mu sync.Mutex
}

var _ exec.Operations = (*mockOperations)(nil)

// DataRead implements [exec.Operations].
func (m *mockOperations) DataRead(ctx context.Context, desired *eval.DesiredResourceInstance, plannedVal cty.Value, providerClient *exec.ProviderClient) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var result *exec.ResourceInstanceObject
	if m.DataReadFunc != nil {
		result, diags = m.DataReadFunc(ctx, desired, plannedVal, providerClient)
	}
	m.appendLog("DataRead", []any{desired, plannedVal, providerClient}, result)
	return result, diags
}

// EphemeralClose implements [exec.Operations].
func (m *mockOperations) EphemeralClose(ctx context.Context, ephemeral *exec.OpenEphemeralResourceInstance) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if m.EphemeralCloseFunc != nil {
		diags = m.EphemeralCloseFunc(ctx, ephemeral)
	}
	m.appendLog("EphemeralClose", []any{ephemeral}, struct{}{})
	return diags
}

// EphemeralOpen implements [exec.Operations].
func (m *mockOperations) EphemeralOpen(ctx context.Context, desired *eval.DesiredResourceInstance, providerClient *exec.ProviderClient) (*exec.OpenEphemeralResourceInstance, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var result *exec.OpenEphemeralResourceInstance
	if m.EphemeralOpenFunc != nil {
		result, diags = m.EphemeralOpenFunc(ctx, desired, providerClient)
	}
	m.appendLog("EphemeralOpen", []any{desired, providerClient}, result)
	return result, diags
}

// EphemeralState implements [exec.Operations].
func (m *mockOperations) EphemeralState(ctx context.Context, ephemeral *exec.OpenEphemeralResourceInstance) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var result *exec.ResourceInstanceObject
	if m.EphemeralStateFunc != nil {
		result, diags = m.EphemeralStateFunc(ctx, ephemeral)
	}
	m.appendLog("EphemeralState", []any{ephemeral}, result)
	return result, diags
}

// ManagedAlreadyDeposed implements [exec.Operations].
func (m *mockOperations) ManagedAlreadyDeposed(ctx context.Context, instAddr addrs.AbsResourceInstance, deposedKey states.DeposedKey) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var result *exec.ResourceInstanceObject
	if m.ManagedAlreadyDeposedFunc != nil {
		result, diags = m.ManagedAlreadyDeposedFunc(ctx, instAddr, deposedKey)
	}
	m.appendLog("ManagedAlreadyDeposed", []any{instAddr, deposedKey}, result)
	return result, diags
}

// ManagedApply implements [exec.Operations].
func (m *mockOperations) ManagedApply(ctx context.Context, plan *exec.ManagedResourceObjectFinalPlan, fallback *exec.ResourceInstanceObject, providerClient *exec.ProviderClient) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var result *exec.ResourceInstanceObject
	if m.ManagedApplyFunc != nil {
		result, diags = m.ManagedApplyFunc(ctx, plan, fallback, providerClient)
	}
	m.appendLog("ManagedApply", []any{plan, fallback, providerClient}, result)
	return result, diags
}

// ManagedChangeAddr implements [exec.Operations].
func (m *mockOperations) ManagedChangeAddr(ctx context.Context, currentObj *exec.ResourceInstanceObject, newAddr addrs.AbsResourceInstance) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var result *exec.ResourceInstanceObject
	if m.ManagedChangeAddrFunc != nil {
		result, diags = m.ManagedChangeAddrFunc(ctx, currentObj, newAddr)
	}
	m.appendLog("ManagedChangeAddr", []any{currentObj, newAddr}, result)
	return result, diags
}

// ManagedDepose implements [exec.Operations].
func (m *mockOperations) ManagedDepose(ctx context.Context, currentObj *exec.ResourceInstanceObject) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var result *exec.ResourceInstanceObject
	if m.ManagedDeposeFunc != nil {
		result, diags = m.ManagedDeposeFunc(ctx, currentObj)
	}
	m.appendLog("ManagedDepose", []any{currentObj}, result)
	return result, diags
}

// ManagedFinalPlan implements [exec.Operations].
func (m *mockOperations) ManagedFinalPlan(ctx context.Context, desired *eval.DesiredResourceInstance, prior *exec.ResourceInstanceObject, plannedVal cty.Value, providerClient *exec.ProviderClient) (*exec.ManagedResourceObjectFinalPlan, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var result *exec.ManagedResourceObjectFinalPlan
	if m.ManagedFinalPlanFunc != nil {
		result, diags = m.ManagedFinalPlanFunc(ctx, desired, prior, plannedVal, providerClient)
	}
	m.appendLog("ManagedFinalPlan", []any{desired, prior, plannedVal, providerClient}, result)
	return result, diags
}

// ProviderInstanceClose implements [exec.Operations].
func (m *mockOperations) ProviderInstanceClose(ctx context.Context, client *exec.ProviderClient) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if m.ProviderInstanceCloseFunc != nil {
		diags = m.ProviderInstanceClose(ctx, client)
	}
	m.appendLog("ProviderInstanceClose", []any{client}, struct{}{})
	return diags
}

// ProviderInstanceConfig implements [exec.Operations].
func (m *mockOperations) ProviderInstanceConfig(ctx context.Context, instAddr addrs.AbsProviderInstanceCorrect) (*exec.ProviderInstanceConfig, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var result *exec.ProviderInstanceConfig
	if m.ProviderInstanceConfigFunc != nil {
		result, diags = m.ProviderInstanceConfigFunc(ctx, instAddr)
	}
	m.appendLog("ProviderInstanceConfig", []any{instAddr}, result)
	return result, diags
}

// ProviderInstanceOpen implements [exec.Operations].
func (m *mockOperations) ProviderInstanceOpen(ctx context.Context, config *exec.ProviderInstanceConfig) (*exec.ProviderClient, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var result *exec.ProviderClient
	if m.ProviderInstanceOpenFunc != nil {
		result, diags = m.ProviderInstanceOpenFunc(ctx, config)
	}
	m.appendLog("ProviderInstanceOpen", []any{config}, result)
	return result, diags
}

// ResourceInstanceDesired implements [exec.Operations].
func (m *mockOperations) ResourceInstanceDesired(ctx context.Context, instAddr addrs.AbsResourceInstance) (*eval.DesiredResourceInstance, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var result *eval.DesiredResourceInstance
	if m.ResourceInstanceDesiredFunc != nil {
		result, diags = m.ResourceInstanceDesiredFunc(ctx, instAddr)
	}
	m.appendLog("ResourceInstanceDesired", []any{instAddr}, result)
	return result, diags
}

// ResourceInstancePostconditions implements [exec.Operations].
func (m *mockOperations) ResourceInstancePostconditions(ctx context.Context, result *exec.ResourceInstanceObject) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if m.ResourceInstancePostconditionsFunc != nil {
		diags = m.ResourceInstancePostconditionsFunc(ctx, result)
	}
	m.appendLog("ResourceInstancePostconditions", []any{result}, result)
	return diags
}

// ResourceInstancePrior implements [exec.Operations].
func (m *mockOperations) ResourceInstancePrior(ctx context.Context, instAddr addrs.AbsResourceInstance) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var result *exec.ResourceInstanceObject
	if m.ResourceInstancePriorFunc != nil {
		result, diags = m.ResourceInstancePriorFunc(ctx, instAddr)
	}
	m.appendLog("ResourceInstancePrior", []any{instAddr}, result)
	return result, diags
}

func (m *mockOperations) NewManagedResourceProviderClient(
	planFunc func(context.Context, providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse,
	applyFunc func(context.Context, providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse,
) providers.Configured {
	return &managedResourceInstanceMockProvider{
		PlanResourceChangeFunc:  planFunc,
		ApplyResourceChangeFunc: applyFunc,
		execCtx:                 m,
	}
}

func (m *mockOperations) appendLog(methodName string, args []any, result any) {
	//log.Printf("[TRACE] execgraph.MockExecContext: %s(%#v) -> %#v", methodName, args, result)
	m.mu.Lock()
	m.Calls = append(m.Calls, mockOperationsCall{
		MethodName: methodName,
		Args:       args,
		Result:     result,
	})
	m.mu.Unlock()
}

type mockOperationsCall struct {
	MethodName string
	Args       []any
	Result     any
}

type managedResourceInstanceMockProvider struct {
	PlanResourceChangeFunc  func(ctx context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse
	ApplyResourceChangeFunc func(ctx context.Context, req providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse
	execCtx                 *mockOperations
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
