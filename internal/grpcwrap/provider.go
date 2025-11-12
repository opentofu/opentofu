// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package grpcwrap

import (
	"context"

	"github.com/opentofu/opentofu/internal/plugin/convert"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfplugin5"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"github.com/zclconf/go-cty/cty/msgpack"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// New wraps a providers.Interface to implement a grpc ProviderServer.
// This is useful for creating a test binary out of an internal provider
// implementation.
func Provider(p providers.Interface) tfplugin5.ProviderServer {
	return &provider{
		provider: p,
		schema:   p.GetProviderSchema(context.TODO()),
	}
}

type provider struct {
	provider providers.Interface
	schema   providers.GetProviderSchemaResponse

	tfplugin5.UnimplementedProviderServer
}

func (p *provider) GetSchema(_ context.Context, req *tfplugin5.GetProviderSchema_Request) (*tfplugin5.GetProviderSchema_Response, error) {
	resp := &tfplugin5.GetProviderSchema_Response{
		ResourceSchemas:          make(map[string]*tfplugin5.Schema),
		DataSourceSchemas:        make(map[string]*tfplugin5.Schema),
		EphemeralResourceSchemas: make(map[string]*tfplugin5.Schema),
	}

	resp.Provider = &tfplugin5.Schema{
		Block: &tfplugin5.Schema_Block{},
	}
	if p.schema.Provider.Block != nil {
		resp.Provider.Block = convert.ConfigSchemaToProto(p.schema.Provider.Block)
	}

	resp.ProviderMeta = &tfplugin5.Schema{
		Block: &tfplugin5.Schema_Block{},
	}
	if p.schema.ProviderMeta.Block != nil {
		resp.ProviderMeta.Block = convert.ConfigSchemaToProto(p.schema.ProviderMeta.Block)
	}

	for typ, res := range p.schema.ResourceTypes {
		resp.ResourceSchemas[typ] = &tfplugin5.Schema{
			Version: res.Version,
			Block:   convert.ConfigSchemaToProto(res.Block),
		}
	}
	for typ, dat := range p.schema.DataSources {
		resp.DataSourceSchemas[typ] = &tfplugin5.Schema{
			Version: dat.Version,
			Block:   convert.ConfigSchemaToProto(dat.Block),
		}
	}
	for typ, dat := range p.schema.EphemeralResources {
		resp.EphemeralResourceSchemas[typ] = &tfplugin5.Schema{
			Version: dat.Version,
			Block:   convert.ConfigSchemaToProto(dat.Block),
		}
	}

	resp.ServerCapabilities = &tfplugin5.ServerCapabilities{
		PlanDestroy: p.schema.ServerCapabilities.PlanDestroy,
	}

	// include any diagnostics from the original GetSchema call
	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, p.schema.Diagnostics)

	return resp, nil
}

func (p *provider) PrepareProviderConfig(ctx context.Context, req *tfplugin5.PrepareProviderConfig_Request) (*tfplugin5.PrepareProviderConfig_Response, error) {
	resp := &tfplugin5.PrepareProviderConfig_Response{}
	ty := p.schema.Provider.Block.ImpliedType()

	configVal, err := decodeDynamicValue(req.Config, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	prepareResp := p.provider.ValidateProviderConfig(ctx, providers.ValidateProviderConfigRequest{
		Config: configVal,
	})

	// the PreparedConfig value is no longer used
	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, prepareResp.Diagnostics)
	return resp, nil
}

func (p *provider) ValidateResourceTypeConfig(ctx context.Context, req *tfplugin5.ValidateResourceTypeConfig_Request) (*tfplugin5.ValidateResourceTypeConfig_Response, error) {
	resp := &tfplugin5.ValidateResourceTypeConfig_Response{}
	ty := p.schema.ResourceTypes[req.TypeName].Block.ImpliedType()

	configVal, err := decodeDynamicValue(req.Config, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	validateResp := p.provider.ValidateResourceConfig(ctx, providers.ValidateResourceConfigRequest{
		TypeName: req.TypeName,
		Config:   configVal,
	})

	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, validateResp.Diagnostics)
	return resp, nil
}

func (p *provider) ValidateDataSourceConfig(ctx context.Context, req *tfplugin5.ValidateDataSourceConfig_Request) (*tfplugin5.ValidateDataSourceConfig_Response, error) {
	resp := &tfplugin5.ValidateDataSourceConfig_Response{}
	ty := p.schema.DataSources[req.TypeName].Block.ImpliedType()

	configVal, err := decodeDynamicValue(req.Config, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	validateResp := p.provider.ValidateDataResourceConfig(ctx, providers.ValidateDataResourceConfigRequest{
		TypeName: req.TypeName,
		Config:   configVal,
	})

	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, validateResp.Diagnostics)
	return resp, nil
}

// ValidateEphemeralResourceConfig implements tfplugin5.ProviderServer.
func (p *provider) ValidateEphemeralResourceConfig(ctx context.Context, req *tfplugin5.ValidateEphemeralResourceConfig_Request) (*tfplugin5.ValidateEphemeralResourceConfig_Response, error) {
	resp := &tfplugin5.ValidateEphemeralResourceConfig_Response{}
	ty := p.schema.EphemeralResources[req.TypeName].Block.ImpliedType()

	configVal, err := decodeDynamicValue(req.Config, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	validateResp := p.provider.ValidateEphemeralConfig(ctx, providers.ValidateEphemeralConfigRequest{
		TypeName: req.TypeName,
		Config:   configVal,
	})

	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, validateResp.Diagnostics)
	return resp, nil
}

func (p *provider) UpgradeResourceState(ctx context.Context, req *tfplugin5.UpgradeResourceState_Request) (*tfplugin5.UpgradeResourceState_Response, error) {
	resp := &tfplugin5.UpgradeResourceState_Response{}
	ty := p.schema.ResourceTypes[req.TypeName].Block.ImpliedType()

	upgradeResp := p.provider.UpgradeResourceState(ctx, providers.UpgradeResourceStateRequest{
		TypeName:     req.TypeName,
		Version:      req.Version,
		RawStateJSON: req.RawState.Json,
	})

	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, upgradeResp.Diagnostics)
	if upgradeResp.Diagnostics.HasErrors() {
		return resp, nil
	}

	dv, err := encodeDynamicValue(upgradeResp.UpgradedState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	resp.UpgradedState = dv

	return resp, nil
}

func (p *provider) Configure(ctx context.Context, req *tfplugin5.Configure_Request) (*tfplugin5.Configure_Response, error) {
	resp := &tfplugin5.Configure_Response{}
	ty := p.schema.Provider.Block.ImpliedType()

	configVal, err := decodeDynamicValue(req.Config, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	configureResp := p.provider.ConfigureProvider(ctx, providers.ConfigureProviderRequest{
		TerraformVersion: req.TerraformVersion,
		Config:           configVal,
	})

	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, configureResp.Diagnostics)
	return resp, nil
}

func (p *provider) ReadResource(ctx context.Context, req *tfplugin5.ReadResource_Request) (*tfplugin5.ReadResource_Response, error) {
	resp := &tfplugin5.ReadResource_Response{}
	ty := p.schema.ResourceTypes[req.TypeName].Block.ImpliedType()

	stateVal, err := decodeDynamicValue(req.CurrentState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	metaTy := p.schema.ProviderMeta.Block.ImpliedType()
	metaVal, err := decodeDynamicValue(req.ProviderMeta, metaTy)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	readResp := p.provider.ReadResource(ctx, providers.ReadResourceRequest{
		TypeName:     req.TypeName,
		PriorState:   stateVal,
		Private:      req.Private,
		ProviderMeta: metaVal,
	})
	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, readResp.Diagnostics)
	if readResp.Diagnostics.HasErrors() {
		return resp, nil
	}
	resp.Private = readResp.Private

	dv, err := encodeDynamicValue(readResp.NewState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}
	resp.NewState = dv

	return resp, nil
}

func (p *provider) PlanResourceChange(ctx context.Context, req *tfplugin5.PlanResourceChange_Request) (*tfplugin5.PlanResourceChange_Response, error) {
	resp := &tfplugin5.PlanResourceChange_Response{}
	ty := p.schema.ResourceTypes[req.TypeName].Block.ImpliedType()

	priorStateVal, err := decodeDynamicValue(req.PriorState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	proposedStateVal, err := decodeDynamicValue(req.ProposedNewState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	configVal, err := decodeDynamicValue(req.Config, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	metaTy := p.schema.ProviderMeta.Block.ImpliedType()
	metaVal, err := decodeDynamicValue(req.ProviderMeta, metaTy)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	planResp := p.provider.PlanResourceChange(ctx, providers.PlanResourceChangeRequest{
		TypeName:         req.TypeName,
		PriorState:       priorStateVal,
		ProposedNewState: proposedStateVal,
		Config:           configVal,
		PriorPrivate:     req.PriorPrivate,
		ProviderMeta:     metaVal,
	})
	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, planResp.Diagnostics)
	if planResp.Diagnostics.HasErrors() {
		return resp, nil
	}

	resp.PlannedPrivate = planResp.PlannedPrivate

	resp.PlannedState, err = encodeDynamicValue(planResp.PlannedState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	for _, path := range planResp.RequiresReplace {
		resp.RequiresReplace = append(resp.RequiresReplace, convert.PathToAttributePath(path))
	}

	return resp, nil
}

func (p *provider) ApplyResourceChange(ctx context.Context, req *tfplugin5.ApplyResourceChange_Request) (*tfplugin5.ApplyResourceChange_Response, error) {
	resp := &tfplugin5.ApplyResourceChange_Response{}
	ty := p.schema.ResourceTypes[req.TypeName].Block.ImpliedType()

	priorStateVal, err := decodeDynamicValue(req.PriorState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	plannedStateVal, err := decodeDynamicValue(req.PlannedState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	configVal, err := decodeDynamicValue(req.Config, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	metaTy := p.schema.ProviderMeta.Block.ImpliedType()
	metaVal, err := decodeDynamicValue(req.ProviderMeta, metaTy)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	applyResp := p.provider.ApplyResourceChange(ctx, providers.ApplyResourceChangeRequest{
		TypeName:       req.TypeName,
		PriorState:     priorStateVal,
		PlannedState:   plannedStateVal,
		Config:         configVal,
		PlannedPrivate: req.PlannedPrivate,
		ProviderMeta:   metaVal,
	})

	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, applyResp.Diagnostics)
	if applyResp.Diagnostics.HasErrors() {
		return resp, nil
	}
	resp.Private = applyResp.Private

	resp.NewState, err = encodeDynamicValue(applyResp.NewState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	return resp, nil
}

func (p *provider) ImportResourceState(ctx context.Context, req *tfplugin5.ImportResourceState_Request) (*tfplugin5.ImportResourceState_Response, error) {
	resp := &tfplugin5.ImportResourceState_Response{}

	importResp := p.provider.ImportResourceState(ctx, providers.ImportResourceStateRequest{
		TypeName: req.TypeName,
		ID:       req.Id,
	})
	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, importResp.Diagnostics)

	for _, res := range importResp.ImportedResources {
		ty := p.schema.ResourceTypes[res.TypeName].Block.ImpliedType()
		state, err := encodeDynamicValue(res.State, ty)
		if err != nil {
			resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
			continue
		}

		resp.ImportedResources = append(resp.ImportedResources, &tfplugin5.ImportResourceState_ImportedResource{
			TypeName: res.TypeName,
			State:    state,
			Private:  res.Private,
		})
	}

	return resp, nil
}

func (p *provider) MoveResourceState(context.Context, *tfplugin5.MoveResourceState_Request) (*tfplugin5.MoveResourceState_Response, error) {
	panic("Not Implemented")
}

func (p *provider) ReadDataSource(ctx context.Context, req *tfplugin5.ReadDataSource_Request) (*tfplugin5.ReadDataSource_Response, error) {
	resp := &tfplugin5.ReadDataSource_Response{}
	ty := p.schema.DataSources[req.TypeName].Block.ImpliedType()

	configVal, err := decodeDynamicValue(req.Config, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	metaTy := p.schema.ProviderMeta.Block.ImpliedType()
	metaVal, err := decodeDynamicValue(req.ProviderMeta, metaTy)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	readResp := p.provider.ReadDataSource(ctx, providers.ReadDataSourceRequest{
		TypeName:     req.TypeName,
		Config:       configVal,
		ProviderMeta: metaVal,
	})
	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, readResp.Diagnostics)
	if readResp.Diagnostics.HasErrors() {
		return resp, nil
	}

	resp.State, err = encodeDynamicValue(readResp.State, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	return resp, nil
}

// OpenEphemeralResource implements tfplugin5.ProviderServer.
func (p *provider) OpenEphemeralResource(ctx context.Context, req *tfplugin5.OpenEphemeralResource_Request) (*tfplugin5.OpenEphemeralResource_Response, error) {
	resp := &tfplugin5.OpenEphemeralResource_Response{}
	ty := p.schema.EphemeralResources[req.TypeName].Block.ImpliedType()

	configVal, err := decodeDynamicValue(req.Config, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	openResp := p.provider.OpenEphemeralResource(ctx, providers.OpenEphemeralResourceRequest{
		TypeName: req.TypeName,
		Config:   configVal,
	})
	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, openResp.Diagnostics)
	if openResp.Diagnostics.HasErrors() {
		return resp, nil
	}

	resp.Result, err = encodeDynamicValue(openResp.Result, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	resp.Private = openResp.Private
	if openResp.RenewAt != nil {
		resp.RenewAt = timestamppb.New(*openResp.RenewAt)
	}
	return resp, nil
}

// RenewEphemeralResource implements tfplugin5.ProviderServer.
func (p *provider) RenewEphemeralResource(ctx context.Context, req *tfplugin5.RenewEphemeralResource_Request) (*tfplugin5.RenewEphemeralResource_Response, error) {
	resp := &tfplugin5.RenewEphemeralResource_Response{}

	renewResp := p.provider.RenewEphemeralResource(ctx, providers.RenewEphemeralResourceRequest{
		TypeName: req.TypeName,
		Private:  req.Private,
	})
	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, renewResp.Diagnostics)
	if renewResp.Diagnostics.HasErrors() {
		return resp, nil
	}

	resp.Private = renewResp.Private
	if renewResp.RenewAt != nil {
		resp.RenewAt = timestamppb.New(*renewResp.RenewAt)
	}
	return resp, nil
}

// CloseEphemeralResource implements tfplugin5.ProviderServer.
func (p *provider) CloseEphemeralResource(ctx context.Context, req *tfplugin5.CloseEphemeralResource_Request) (*tfplugin5.CloseEphemeralResource_Response, error) {
	resp := &tfplugin5.CloseEphemeralResource_Response{}

	renewResp := p.provider.CloseEphemeralResource(ctx, providers.CloseEphemeralResourceRequest{
		TypeName: req.TypeName,
		Private:  req.Private,
	})
	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, renewResp.Diagnostics)
	return resp, nil
}

func (p *provider) Stop(ctx context.Context, _ *tfplugin5.Stop_Request) (*tfplugin5.Stop_Response, error) {
	resp := &tfplugin5.Stop_Response{}
	err := p.provider.Stop(ctx)
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

func (p *provider) GetFunctions(context.Context, *tfplugin5.GetFunctions_Request) (*tfplugin5.GetFunctions_Response, error) {
	panic("Not Implemented")
}

func (p *provider) CallFunction(context.Context, *tfplugin5.CallFunction_Request) (*tfplugin5.CallFunction_Response, error) {
	panic("Not Implemented")
}

// GetResourceIdentitySchemas implements tfplugin5.ProviderServer.
func (p *provider) GetResourceIdentitySchemas(context.Context, *tfplugin5.GetResourceIdentitySchemas_Request) (*tfplugin5.GetResourceIdentitySchemas_Response, error) {
	panic("unimplemented")
}

// UpgradeResourceIdentity implements tfplugin5.ProviderServer.
func (p *provider) UpgradeResourceIdentity(context.Context, *tfplugin5.UpgradeResourceIdentity_Request) (*tfplugin5.UpgradeResourceIdentity_Response, error) {
	panic("unimplemented")
}

func (p *provider) GetMetadata(context.Context, *tfplugin5.GetMetadata_Request) (*tfplugin5.GetMetadata_Response, error) {
	panic("Not Implemented")
}

// decode a DynamicValue from either the JSON or MsgPack encoding.
func decodeDynamicValue(v *tfplugin5.DynamicValue, ty cty.Type) (cty.Value, error) {
	// always return a valid value
	var err error
	res := cty.NullVal(ty)
	if v == nil {
		return res, nil
	}

	switch {
	case len(v.Msgpack) > 0:
		res, err = msgpack.Unmarshal(v.Msgpack, ty)
	case len(v.Json) > 0:
		res, err = ctyjson.Unmarshal(v.Json, ty)
	}
	return res, err
}

// encode a cty.Value into a DynamicValue msgpack payload.
func encodeDynamicValue(v cty.Value, ty cty.Type) (*tfplugin5.DynamicValue, error) {
	mp, err := msgpack.Marshal(v, ty)
	return &tfplugin5.DynamicValue{
		Msgpack: mp,
	}, err
}
