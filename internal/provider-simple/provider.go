// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// simple provider a minimal provider implementation for testing
package simple

import (
	"context"
	"errors"
	"time"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

type simple struct {
	schema providers.GetProviderSchemaResponse
}

func Provider() providers.Interface {
	simpleResource := providers.Schema{
		Block: &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"id": {
					Computed: true,
					Type:     cty.String,
				},
				"value": {
					Optional: true,
					Type:     cty.String,
				},
			},
		},
	}

	return simple{
		schema: providers.GetProviderSchemaResponse{
			Provider: providers.Schema{
				Block: nil,
			},
			ResourceTypes: map[string]providers.Schema{
				"simple_resource": simpleResource,
			},
			DataSources: map[string]providers.Schema{
				"simple_resource": simpleResource,
			},
			ServerCapabilities: providers.ServerCapabilities{
				PlanDestroy: true,
			},
		},
	}
}

func (s simple) GetProviderSchema(_ context.Context) providers.GetProviderSchemaResponse {
	return s.schema
}

func (s simple) ValidateProviderConfig(_ context.Context, req providers.ValidateProviderConfigRequest) (resp providers.ValidateProviderConfigResponse) {
	return resp
}

func (s simple) ValidateResourceConfig(_ context.Context, req providers.ValidateResourceConfigRequest) (resp providers.ValidateResourceConfigResponse) {
	return resp
}

func (s simple) ValidateDataResourceConfig(_ context.Context, req providers.ValidateDataResourceConfigRequest) (resp providers.ValidateDataResourceConfigResponse) {
	return resp
}

func (s simple) MoveResourceState(_ context.Context, req providers.MoveResourceStateRequest) providers.MoveResourceStateResponse {
	var resp providers.MoveResourceStateResponse
	val, err := ctyjson.Unmarshal(req.SourceStateJSON, s.schema.ResourceTypes["simple_resource"].Block.ImpliedType())
	resp.Diagnostics = resp.Diagnostics.Append(err)
	if err != nil {
		return resp
	}
	resp.TargetState = val
	resp.TargetPrivate = req.SourcePrivate
	return resp
}
func (s simple) UpgradeResourceState(_ context.Context, req providers.UpgradeResourceStateRequest) providers.UpgradeResourceStateResponse {
	var resp providers.UpgradeResourceStateResponse
	ty := s.schema.ResourceTypes[req.TypeName].Block.ImpliedType()
	val, err := ctyjson.Unmarshal(req.RawStateJSON, ty)
	resp.Diagnostics = resp.Diagnostics.Append(err)
	resp.UpgradedState = val
	return resp
}

func (s simple) ConfigureProvider(context.Context, providers.ConfigureProviderRequest) (resp providers.ConfigureProviderResponse) {
	return resp
}

func (s simple) Stop(_ context.Context) error {
	return nil
}

func (s simple) ReadResource(_ context.Context, req providers.ReadResourceRequest) (resp providers.ReadResourceResponse) {
	// just return the same state we received
	resp.NewState = req.PriorState
	return resp
}

func (s simple) PlanResourceChange(_ context.Context, req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
	if req.ProposedNewState.IsNull() {
		// destroy op
		resp.PlannedState = req.ProposedNewState
		resp.PlannedPrivate = req.PriorPrivate
		return resp
	}

	m := req.ProposedNewState.AsValueMap()
	_, ok := m["id"]
	if !ok {
		m["id"] = cty.UnknownVal(cty.String)
	}

	resp.PlannedState = cty.ObjectVal(m)
	return resp
}

func (s simple) ApplyResourceChange(_ context.Context, req providers.ApplyResourceChangeRequest) (resp providers.ApplyResourceChangeResponse) {
	if req.PlannedState.IsNull() {
		resp.NewState = req.PlannedState
		return resp
	}

	m := req.PlannedState.AsValueMap()
	_, ok := m["id"]
	if !ok {
		m["id"] = cty.StringVal(time.Now().String())
	}
	resp.NewState = cty.ObjectVal(m)

	return resp
}

func (s simple) ImportResourceState(context.Context, providers.ImportResourceStateRequest) (resp providers.ImportResourceStateResponse) {
	resp.Diagnostics = resp.Diagnostics.Append(errors.New("unsupported"))
	return resp
}

func (s simple) ReadDataSource(_ context.Context, req providers.ReadDataSourceRequest) (resp providers.ReadDataSourceResponse) {
	m := req.Config.AsValueMap()
	m["id"] = cty.StringVal("static_id")
	resp.State = cty.ObjectVal(m)
	return resp
}

func (s simple) GetFunctions(_ context.Context) providers.GetFunctionsResponse {
	panic("Not Implemented")
}

func (s simple) CallFunction(_ context.Context, r providers.CallFunctionRequest) providers.CallFunctionResponse {
	panic("Not Implemented")
}

func (s simple) Close(_ context.Context) error {
	return nil
}
