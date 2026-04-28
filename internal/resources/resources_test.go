// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"

	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// fakeProviderClient is an implementation of [providers.Interface] that just
// delegates to various function pointers in its own fields, so that we can more
// easily unit test the methods in this package that make calls to these
// methods and then react to what they return.
//
// This is intentionally simpler than the mock provider implementation used
// in "package tofu"'s context tests: there's no tracking of what was called
// and what arguments were provided, so tests which need that will need to
// implement it themselves e.g. by using closures that capture local variables
// from the test function's scope.
type fakeProviderClient struct {
	// Schema is a pointer to a value that is to be returned from
	// [fakeProviderClient.GetProviderSchema].
	//
	// If this is nil then that function just returns an empty schema.
	schema *providers.GetProviderSchemaResponse

	// validateResourceConfig is the implementation of [fakeProviderClient.ValidateResourceConfig].
	//
	// If this is left nil then the default implementation just considers all
	// configurations to be valid.
	validateResourceConfig func(context.Context, providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse

	// readResource is the implementation of [fakeProviderClient.ReadResource].
	//
	// If this is left nil then the default implementation immediately returns
	// an error, because there's no reasonable default behavior.
	readResource func(context.Context, providers.ReadResourceRequest) providers.ReadResourceResponse

	// planResourceChange is the implementation of [fakeProviderClient.PlanResourceChange].
	//
	// If this is left nil then the default implementation just echoes back
	// the proposed new value.
	planResourceChange func(context.Context, providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse

	// applyResourceChange is the implementation of [fakeProviderClient.ApplyResourceChange].
	//
	// If this is left nil then the default implementation just echoes back
	// the planned value.
	applyResourceChange func(context.Context, providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse
}

var _ providers.Interface = (*fakeProviderClient)(nil)

// GetProviderSchema implements [providers.Interface].
func (f *fakeProviderClient) GetProviderSchema(context.Context) providers.GetProviderSchemaResponse {
	if f.schema == nil {
		return providers.GetProviderSchemaResponse{
			// This schema intentionally left blank. If your test needs a schema
			// then it should write a non-nil pointer into the Schema field.
		}
	}
	return *f.schema
}

// ValidateResourceConfig implements [providers.Interface].
func (f *fakeProviderClient) ValidateResourceConfig(ctx context.Context, req providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse {
	if f.validateResourceConfig == nil {
		return providers.ValidateResourceConfigResponse{
			Diagnostics: nil,
		}
	}
	return f.validateResourceConfig(ctx, req)
}

// ReadResource implements [providers.Interface].
func (f *fakeProviderClient) ReadResource(ctx context.Context, req providers.ReadResourceRequest) providers.ReadResourceResponse {
	if f.readResource == nil {
		var diags tfdiags.Diagnostics
		diags = diags.Append(fmt.Errorf("this fakeProviderClient does not support ReadResource"))
		return providers.ReadResourceResponse{
			Diagnostics: diags,
		}
	}
	return f.readResource(ctx, req)
}

// PlanResourceChange implements [providers.Interface].
func (f *fakeProviderClient) PlanResourceChange(ctx context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
	if f.planResourceChange == nil {
		return providers.PlanResourceChangeResponse{
			PlannedState: req.ProposedNewState,
		}
	}
	return f.planResourceChange(ctx, req)
}

// ApplyResourceChange implements [providers.Interface].
func (f *fakeProviderClient) ApplyResourceChange(ctx context.Context, req providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
	if f.applyResourceChange == nil {
		return providers.ApplyResourceChangeResponse{
			NewState: req.PlannedState,
		}
	}
	return f.applyResourceChange(ctx, req)
}

// UpgradeResourceIdentity implements [providers.Interface].
func (f *fakeProviderClient) UpgradeResourceIdentity(context.Context, providers.UpgradeResourceIdentityRequest) providers.UpgradeResourceIdentityResponse {
	panic("unimplemented")
}

// UpgradeResourceState implements [providers.Interface].
func (f *fakeProviderClient) UpgradeResourceState(context.Context, providers.UpgradeResourceStateRequest) providers.UpgradeResourceStateResponse {
	panic("unimplemented")
}

// ImportResourceState implements [providers.Interface].
func (f *fakeProviderClient) ImportResourceState(context.Context, providers.ImportResourceStateRequest) providers.ImportResourceStateResponse {
	panic("unimplemented")
}

// MoveResourceState implements [providers.Interface].
func (f *fakeProviderClient) MoveResourceState(context.Context, providers.MoveResourceStateRequest) providers.MoveResourceStateResponse {
	panic("unimplemented")
}

// ValidateDataResourceConfig implements [providers.Interface].
func (f *fakeProviderClient) ValidateDataResourceConfig(context.Context, providers.ValidateDataResourceConfigRequest) providers.ValidateDataResourceConfigResponse {
	panic("unimplemented")
}

// ReadDataSource implements [providers.Interface].
func (f *fakeProviderClient) ReadDataSource(context.Context, providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
	panic("unimplemented")
}

// ValidateEphemeralConfig implements [providers.Interface].
func (f *fakeProviderClient) ValidateEphemeralConfig(context.Context, providers.ValidateEphemeralConfigRequest) providers.ValidateEphemeralConfigResponse {
	panic("unimplemented")
}

// OpenEphemeralResource implements [providers.Interface].
func (f *fakeProviderClient) OpenEphemeralResource(context.Context, providers.OpenEphemeralResourceRequest) providers.OpenEphemeralResourceResponse {
	panic("unimplemented")
}

// RenewEphemeralResource implements [providers.Interface].
func (f *fakeProviderClient) RenewEphemeralResource(context.Context, providers.RenewEphemeralResourceRequest) (resp providers.RenewEphemeralResourceResponse) {
	panic("unimplemented")
}

// CloseEphemeralResource implements [providers.Interface].
func (f *fakeProviderClient) CloseEphemeralResource(context.Context, providers.CloseEphemeralResourceRequest) providers.CloseEphemeralResourceResponse {
	panic("unimplemented")
}

// Close implements [providers.Interface].
func (f *fakeProviderClient) Close(context.Context) error {
	return nil
}

// Stop implements [providers.Interface].
func (f *fakeProviderClient) Stop(context.Context) error {
	return nil
}

// ValidateProviderConfig implements [providers.Interface].
func (f *fakeProviderClient) ValidateProviderConfig(context.Context, providers.ValidateProviderConfigRequest) providers.ValidateProviderConfigResponse {
	// We don't support [ConfigureProvider], so we have no need to validate.
	var diags tfdiags.Diagnostics
	diags = diags.Append(fmt.Errorf("fakeProviderClient does not support ValidateProviderConfig"))
	return providers.ValidateProviderConfigResponse{
		Diagnostics: diags,
	}
}

// ConfigureProvider implements [providers.Interface].
func (f *fakeProviderClient) ConfigureProvider(context.Context, providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
	// The methods in this package either work with unconfigured provider
	// clients or provider clients that were preconfigured by the caller, so
	// we don't expect to ever be calling ConfigureProvider ourselves in here.
	var diags tfdiags.Diagnostics
	diags = diags.Append(fmt.Errorf("fakeProviderClient does not support ConfigureProvider"))
	return providers.ConfigureProviderResponse{
		Diagnostics: diags,
	}
}

// GetFunctions implements [providers.Interface].
func (f *fakeProviderClient) GetFunctions(context.Context) providers.GetFunctionsResponse {
	var diags tfdiags.Diagnostics
	diags = diags.Append(fmt.Errorf("fakeProviderClient does not support GetFunctions"))
	return providers.GetFunctionsResponse{
		Diagnostics: diags,
	}
}

// CallFunction implements [providers.Interface].
func (f *fakeProviderClient) CallFunction(context.Context, providers.CallFunctionRequest) providers.CallFunctionResponse {
	return providers.CallFunctionResponse{
		Error: fmt.Errorf("fakeProviderClient does not support CallFunction"),
	}
}
