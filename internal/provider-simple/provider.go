// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// simple provider a minimal provider implementation for testing
package simple

import (
	"context"
	"errors"
	"strings"
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
	// Only managed resource should have write-only arguments.
	withWriteOnlyAttribute := func(s providers.Schema) providers.Schema {
		b := *s.Block

		b.Attributes["value_wo"] = &configschema.Attribute{
			Optional:  true,
			Type:      cty.String,
			WriteOnly: true,
		}
		return providers.Schema{Block: &b}
	}

	return simple{
		schema: providers.GetProviderSchemaResponse{
			Provider: providers.Schema{
				// The "i_depend_on" field is just a simple configuration attribute of the provider
				// to allow creation of dependencies between a resources from a previously
				// initialized provider and this provider.
				// The "i_depend_on" field is having no functionality behind, in the provider context,
				// but it's just a way for the "provider" block to create depedencies
				// to other blocks.
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"i_depend_on": {
							Type:        cty.String,
							Description: "Non-functional configuration attribute of the provider. This is meant to be used only to create depedencies of other resources to the provider block",
							Optional:    true,
						},
					},
				},
			},
			ResourceTypes: map[string]providers.Schema{
				"simple_resource": withWriteOnlyAttribute(simpleResource),
			},
			DataSources: map[string]providers.Schema{
				"simple_resource": simpleResource,
			},
			EphemeralResources: map[string]providers.Schema{
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

func (s simple) ValidateEphemeralConfig(context.Context, providers.ValidateEphemeralConfigRequest) (resp providers.ValidateEphemeralConfigResponse) {
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

	// TODO ephemeral - remove this line after work will be done on write-only arguments.
	// The problem now is that the value sent to ApplyResourceChange is always null as returned by the plan call.
	// When the work on write-only arguments will be done, OpenTofu should send the actual value to
	// the ApplyResourceChange too.
	// To confirm that everything is ok, by removing this "waitIfRequested" call from here, theTestEphemeralWorkflowAndOutput
	// should still work correctly without any warn logs in the test output
	waitIfRequested(m)
	// Simulate what the terraform-plugin-go should do. Nullify the write-only attributes.
	m["value_wo"] = cty.NullVal(cty.String)

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
	waitIfRequested(m)

	// Simulate what the terraform-plugin-go should do. Nullify the write-only attributes.
	m["value_wo"] = cty.NullVal(cty.String)
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

func (s simple) OpenEphemeralResource(_ context.Context, request providers.OpenEphemeralResourceRequest) (resp providers.OpenEphemeralResourceResponse) {
	m := request.Config.AsValueMap()
	m["id"] = cty.StringVal("static-ephemeral-id")
	if v, ok := m["value"]; ok && !v.IsNull() && strings.Contains(v.AsString(), "with-renew") {
		t := time.Now().Add(200 * time.Millisecond)
		resp.RenewAt = &t
	}
	resp.Result = cty.ObjectVal(m)
	resp.Private = []byte("static private data")
	return resp
}

func (s simple) RenewEphemeralResource(_ context.Context, request providers.RenewEphemeralResourceRequest) (resp providers.RenewEphemeralResourceResponse) {
	resp.Private = request.Private
	t := time.Now().Add(200 * time.Millisecond)
	resp.RenewAt = &t
	return resp
}

func (s simple) CloseEphemeralResource(_ context.Context, _ providers.CloseEphemeralResourceRequest) (resp providers.CloseEphemeralResourceResponse) {
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

func waitIfRequested(m map[string]cty.Value) {
	// This is a special case that can be used together with ephemeral resources to be able to test the renewal process.
	// When the "value" attribute of the resource is containing "with-renew" it will return later to allow
	// the ephemeral resource to call renew at least once. Check also OpenEphemeralResource.
	if v, ok := m["value_wo"]; ok && !v.IsNull() && strings.Contains(v.AsString(), "with-renew") {
		<-time.After(time.Second)
	}
}
