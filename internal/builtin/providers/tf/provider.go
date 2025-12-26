// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tf

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
)

// Provider is an implementation of providers.Interface
type Provider struct {
	funcs map[string]providerFunc
}

// NewProvider returns a new tofu provider
func NewProvider() providers.Interface {
	return &Provider{
		funcs: getProviderFuncs(),
	}
}

func (p *Provider) getFunctionSpecs() map[string]providers.FunctionSpec {
	funcSpecs := make(map[string]providers.FunctionSpec)
	for name, fn := range p.funcs {
		funcSpecs[name] = fn.GetFunctionSpec()
	}
	return funcSpecs
}

// GetSchema returns the complete schema for the provider.
func (p *Provider) GetProviderSchema(_ context.Context) providers.GetProviderSchemaResponse {
	return providers.GetProviderSchemaResponse{
		DataSources: map[string]providers.Schema{
			"terraform_remote_state": dataSourceRemoteStateGetSchema(),
		},
		ResourceTypes: map[string]providers.Schema{
			"terraform_data": dataStoreResourceSchema(),
		},
		Functions: p.getFunctionSpecs(),
	}
}

// ValidateProviderConfig is used to validate the configuration values.
func (p *Provider) ValidateProviderConfig(_ context.Context, req providers.ValidateProviderConfigRequest) providers.ValidateProviderConfigResponse {
	// At this moment there is nothing to configure for the tofu provider,
	// so we will happily return without taking any action
	var res providers.ValidateProviderConfigResponse
	res.PreparedConfig = req.Config
	return res
}

// ValidateDataResourceConfig is used to validate the data source configuration values.
func (p *Provider) ValidateDataResourceConfig(_ context.Context, req providers.ValidateDataResourceConfigRequest) providers.ValidateDataResourceConfigResponse {

	var res providers.ValidateDataResourceConfigResponse

	// This should not happen
	if req.TypeName != "terraform_remote_state" {
		res.Diagnostics.Append(fmt.Errorf("Error: unsupported data source %s", req.TypeName))
		return res
	}

	diags := dataSourceRemoteStateValidate(req.Config)
	res.Diagnostics = diags

	return res
}

// ValidateEphemeralConfig is used to validate the ephemeral resource configuration values.
func (p *Provider) ValidateEphemeralConfig(context.Context, providers.ValidateEphemeralConfigRequest) providers.ValidateEphemeralConfigResponse {
	panic("Should not be called directly, special case for terraform_remote_state")
}

// Configure configures and initializes the provider.
func (p *Provider) ConfigureProvider(context.Context, providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
	// At this moment there is nothing to configure for the terraform provider,
	// so we will happily return without taking any action
	var res providers.ConfigureProviderResponse
	return res
}

// ReadDataSource returns the data source's current state.
func (p *Provider) ReadDataSource(_ context.Context, req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
	panic("Should not be called directly, special case for terraform_remote_state")
}

func (p *Provider) ReadDataSourceEncrypted(ctx context.Context, req providers.ReadDataSourceRequest, path addrs.AbsResourceInstance, enc encryption.Encryption) providers.ReadDataSourceResponse {
	// call function
	var res providers.ReadDataSourceResponse

	// This should not happen
	if req.TypeName != "terraform_remote_state" {
		res.Diagnostics.Append(fmt.Errorf("Error: unsupported data source %s", req.TypeName))
		return res
	}

	// These string manipulations are kind of funky
	key := path.String()

	// data.terraform_remote_state.foo[4] -> foo[4]
	// module.submod[1].data.terraform_remote_state.bar -> module.submod[1].bar
	key = strings.Replace(key, "data.terraform_remote_state.", "", 1)

	// module.submod[1].bar -> submod[1].bar
	key = strings.TrimPrefix(key, "module.")

	log.Printf("[DEBUG] accessing remote state at %s", key)

	newState, diags := dataSourceRemoteStateRead(ctx, req.Config, enc.RemoteState(key), path)

	if diags.HasErrors() {
		diags = diags.Append(fmt.Errorf("%s: Unable to read remote state", path.String()))
	}

	res.State = newState
	res.Diagnostics = diags

	return res
}

// OpenEphemeralResource opens an ephemeral resource returning the ephemeral value returned from the provider.
func (p *Provider) OpenEphemeralResource(context.Context, providers.OpenEphemeralResourceRequest) providers.OpenEphemeralResourceResponse {
	panic("Should not be called directly, special case for terraform_remote_state")
}

// RenewEphemeralResource is renewing an ephemeral resource returning only the private information from the provider.
func (p *Provider) RenewEphemeralResource(context.Context, providers.RenewEphemeralResourceRequest) providers.RenewEphemeralResourceResponse {
	panic("Should not be called directly, special case for terraform_remote_state")
}

// CloseEphemeralResource is closing an ephemeral resource to allow the provider to clean up any possible remote information
// bound to the previously opened ephemeral resource.
func (p *Provider) CloseEphemeralResource(context.Context, providers.CloseEphemeralResourceRequest) providers.CloseEphemeralResourceResponse {
	panic("Should not be called directly, special case for terraform_remote_state")
}

// Stop is called when the provider should halt any in-flight actions.
func (p *Provider) Stop(_ context.Context) error {
	log.Println("[DEBUG] terraform provider cannot Stop")
	return nil
}

// All the Resource-specific functions are below.
// The terraform provider supplies a single data source, `terraform_remote_state`
// and no resources.

// UpgradeResourceState is called when the state loader encounters an
// instance state whose schema version is less than the one reported by the
// currently-used version of the corresponding provider, and the upgraded
// result is used for any further processing.
func (p *Provider) UpgradeResourceState(_ context.Context, req providers.UpgradeResourceStateRequest) providers.UpgradeResourceStateResponse {
	return upgradeDataStoreResourceState(req)
}

// ReadResource refreshes a resource and returns its current state.
func (p *Provider) ReadResource(_ context.Context, req providers.ReadResourceRequest) providers.ReadResourceResponse {
	return readDataStoreResourceState(req)
}

// PlanResourceChange takes the current state and proposed state of a
// resource, and returns the planned final state.
func (p *Provider) PlanResourceChange(_ context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
	return planDataStoreResourceChange(req)
}

// ApplyResourceChange takes the planned state for a resource, which may
// yet contain unknown computed values, and applies the changes returning
// the final state.
func (p *Provider) ApplyResourceChange(_ context.Context, req providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
	return applyDataStoreResourceChange(req)
}

// ImportResourceState requests that the given resource be imported.
func (p *Provider) ImportResourceState(_ context.Context, req providers.ImportResourceStateRequest) providers.ImportResourceStateResponse {
	if req.TypeName == "terraform_data" {
		return importDataStore(req)
	}

	panic("unimplemented - terraform_remote_state has no resources")
}

// MoveResourceState is called when the state loader encounters an instance state
// that has been moved to a new type, and the state should be updated to reflect the change.
// This is used to move the old state to the new schema.
func (p *Provider) MoveResourceState(_ context.Context, r providers.MoveResourceStateRequest) (resp providers.MoveResourceStateResponse) {
	return moveDataStoreResourceState(r)
}

// ValidateResourceConfig is used to validate the resource configuration values.
func (p *Provider) ValidateResourceConfig(_ context.Context, req providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse {
	return validateDataStoreResourceConfig(req)
}

func (p *Provider) GetFunctions(_ context.Context) providers.GetFunctionsResponse {
	return providers.GetFunctionsResponse{
		Functions: p.getFunctionSpecs(),
	}
}

func (p *Provider) CallFunction(_ context.Context, r providers.CallFunctionRequest) providers.CallFunctionResponse {
	fn, ok := p.funcs[r.Name]
	if !ok {
		return providers.CallFunctionResponse{
			Error: fmt.Errorf("provider function %q not found", r.Name),
		}
	}
	v, err := fn.Call(r.Arguments)
	return providers.CallFunctionResponse{
		Result: v,
		Error:  err,
	}
}

// Close is a noop for this provider, since it's run in-process.
func (p *Provider) Close(_ context.Context) error {
	return nil
}

// providerFunc is an interface representing a built-in provider function
type providerFunc interface {
	// Name returns the name of the function which is used to call it
	Name() string
	// GetFunctionSpec returns the provider function specification
	GetFunctionSpec() providers.FunctionSpec
	// Call is used to invoke the function
	Call(args []cty.Value) (cty.Value, error)
}

// getProviderFuncs returns a map of functions that are registered in the provider
func getProviderFuncs() map[string]providerFunc {
	decodeTFVars := &decodeTFVarsFunc{}
	encodeTFVars := &encodeTFVarsFunc{}
	encodeExpr := &encodeExprFunc{}
	return map[string]providerFunc{
		decodeTFVars.Name(): decodeTFVars,
		encodeTFVars.Name(): encodeTFVars,
		encodeExpr.Name():   encodeExpr,
	}
}
