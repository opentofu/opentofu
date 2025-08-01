// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package grpcwrap

import (
	"context"

	"github.com/opentofu/opentofu/internal/plugin6/convert"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfplugin6"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"github.com/zclconf/go-cty/cty/msgpack"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// New wraps a providers.Interface to implement a grpc ProviderServer using
// plugin protocol v6. This is useful for creating a test binary out of an
// internal provider implementation.
func Provider6(p providers.Interface) tfplugin6.ProviderServer {
	return &provider6{
		provider: p,
		schema:   p.GetProviderSchema(context.TODO()),
	}
}

type provider6 struct {
	provider providers.Interface
	schema   providers.GetProviderSchemaResponse
}

func (p *provider6) GetProviderSchema(_ context.Context, req *tfplugin6.GetProviderSchema_Request) (*tfplugin6.GetProviderSchema_Response, error) {
	resp := &tfplugin6.GetProviderSchema_Response{
		ResourceSchemas:          make(map[string]*tfplugin6.Schema),
		DataSourceSchemas:        make(map[string]*tfplugin6.Schema),
		EphemeralResourceSchemas: make(map[string]*tfplugin6.Schema),
	}

	resp.Provider = &tfplugin6.Schema{
		Block: &tfplugin6.Schema_Block{},
	}
	if p.schema.Provider.Block != nil {
		resp.Provider.Block = convert.ConfigSchemaToProto(p.schema.Provider.Block)
	}

	resp.ProviderMeta = &tfplugin6.Schema{
		Block: &tfplugin6.Schema_Block{},
	}
	if p.schema.ProviderMeta.Block != nil {
		resp.ProviderMeta.Block = convert.ConfigSchemaToProto(p.schema.ProviderMeta.Block)
	}

	for typ, res := range p.schema.ResourceTypes {
		resp.ResourceSchemas[typ] = &tfplugin6.Schema{
			Version: res.Version,
			Block:   convert.ConfigSchemaToProto(res.Block),
		}
	}
	for typ, dat := range p.schema.DataSources {
		resp.DataSourceSchemas[typ] = &tfplugin6.Schema{
			Version: dat.Version,
			Block:   convert.ConfigSchemaToProto(dat.Block),
		}
	}
	for typ, dat := range p.schema.EphemeralResources {
		resp.EphemeralResourceSchemas[typ] = &tfplugin6.Schema{
			Version: dat.Version,
			Block:   convert.ConfigSchemaToProto(dat.Block),
		}
	}

	resp.ServerCapabilities = &tfplugin6.ServerCapabilities{
		PlanDestroy: p.schema.ServerCapabilities.PlanDestroy,
	}

	// include any diagnostics from the original GetSchema call
	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, p.schema.Diagnostics)

	return resp, nil
}

func (p *provider6) ValidateProviderConfig(ctx context.Context, req *tfplugin6.ValidateProviderConfig_Request) (*tfplugin6.ValidateProviderConfig_Response, error) {
	resp := &tfplugin6.ValidateProviderConfig_Response{}
	ty := p.schema.Provider.Block.ImpliedType()

	configVal, err := decodeDynamicValue6(req.Config, ty)
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

func (p *provider6) ValidateResourceConfig(ctx context.Context, req *tfplugin6.ValidateResourceConfig_Request) (*tfplugin6.ValidateResourceConfig_Response, error) {
	resp := &tfplugin6.ValidateResourceConfig_Response{}
	ty := p.schema.ResourceTypes[req.TypeName].Block.ImpliedType()

	configVal, err := decodeDynamicValue6(req.Config, ty)
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

func (p *provider6) ValidateDataResourceConfig(ctx context.Context, req *tfplugin6.ValidateDataResourceConfig_Request) (*tfplugin6.ValidateDataResourceConfig_Response, error) {
	resp := &tfplugin6.ValidateDataResourceConfig_Response{}
	ty := p.schema.DataSources[req.TypeName].Block.ImpliedType()

	configVal, err := decodeDynamicValue6(req.Config, ty)
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

// ValidateEphemeralResourceConfig implements tfplugin6.ProviderServer.
func (p *provider6) ValidateEphemeralResourceConfig(ctx context.Context, req *tfplugin6.ValidateEphemeralResourceConfig_Request) (*tfplugin6.ValidateEphemeralResourceConfig_Response, error) {
	resp := &tfplugin6.ValidateEphemeralResourceConfig_Response{}
	ty := p.schema.EphemeralResources[req.TypeName].Block.ImpliedType()

	configVal, err := decodeDynamicValue6(req.Config, ty)
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

func (p *provider6) UpgradeResourceState(ctx context.Context, req *tfplugin6.UpgradeResourceState_Request) (*tfplugin6.UpgradeResourceState_Response, error) {
	resp := &tfplugin6.UpgradeResourceState_Response{}
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

	dv, err := encodeDynamicValue6(upgradeResp.UpgradedState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	resp.UpgradedState = dv

	return resp, nil
}

func (p *provider6) ConfigureProvider(ctx context.Context, req *tfplugin6.ConfigureProvider_Request) (*tfplugin6.ConfigureProvider_Response, error) {
	resp := &tfplugin6.ConfigureProvider_Response{}
	ty := p.schema.Provider.Block.ImpliedType()

	configVal, err := decodeDynamicValue6(req.Config, ty)
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

func (p *provider6) ReadResource(ctx context.Context, req *tfplugin6.ReadResource_Request) (*tfplugin6.ReadResource_Response, error) {
	resp := &tfplugin6.ReadResource_Response{}
	ty := p.schema.ResourceTypes[req.TypeName].Block.ImpliedType()

	stateVal, err := decodeDynamicValue6(req.CurrentState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	metaTy := p.schema.ProviderMeta.Block.ImpliedType()
	metaVal, err := decodeDynamicValue6(req.ProviderMeta, metaTy)
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

	dv, err := encodeDynamicValue6(readResp.NewState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}
	resp.NewState = dv

	return resp, nil
}

func (p *provider6) PlanResourceChange(ctx context.Context, req *tfplugin6.PlanResourceChange_Request) (*tfplugin6.PlanResourceChange_Response, error) {
	resp := &tfplugin6.PlanResourceChange_Response{}
	ty := p.schema.ResourceTypes[req.TypeName].Block.ImpliedType()

	priorStateVal, err := decodeDynamicValue6(req.PriorState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	proposedStateVal, err := decodeDynamicValue6(req.ProposedNewState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	configVal, err := decodeDynamicValue6(req.Config, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	metaTy := p.schema.ProviderMeta.Block.ImpliedType()
	metaVal, err := decodeDynamicValue6(req.ProviderMeta, metaTy)
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

	resp.PlannedState, err = encodeDynamicValue6(planResp.PlannedState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	for _, path := range planResp.RequiresReplace {
		resp.RequiresReplace = append(resp.RequiresReplace, convert.PathToAttributePath(path))
	}

	return resp, nil
}

func (p *provider6) ApplyResourceChange(ctx context.Context, req *tfplugin6.ApplyResourceChange_Request) (*tfplugin6.ApplyResourceChange_Response, error) {
	resp := &tfplugin6.ApplyResourceChange_Response{}
	ty := p.schema.ResourceTypes[req.TypeName].Block.ImpliedType()

	priorStateVal, err := decodeDynamicValue6(req.PriorState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	plannedStateVal, err := decodeDynamicValue6(req.PlannedState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	configVal, err := decodeDynamicValue6(req.Config, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	metaTy := p.schema.ProviderMeta.Block.ImpliedType()
	metaVal, err := decodeDynamicValue6(req.ProviderMeta, metaTy)
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

	resp.NewState, err = encodeDynamicValue6(applyResp.NewState, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	return resp, nil
}

func (p *provider6) ImportResourceState(ctx context.Context, req *tfplugin6.ImportResourceState_Request) (*tfplugin6.ImportResourceState_Response, error) {
	resp := &tfplugin6.ImportResourceState_Response{}

	importResp := p.provider.ImportResourceState(ctx, providers.ImportResourceStateRequest{
		TypeName: req.TypeName,
		ID:       req.Id,
	})
	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, importResp.Diagnostics)

	for _, res := range importResp.ImportedResources {
		ty := p.schema.ResourceTypes[res.TypeName].Block.ImpliedType()
		state, err := encodeDynamicValue6(res.State, ty)
		if err != nil {
			resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
			continue
		}

		resp.ImportedResources = append(resp.ImportedResources, &tfplugin6.ImportResourceState_ImportedResource{
			TypeName: res.TypeName,
			State:    state,
			Private:  res.Private,
		})
	}

	return resp, nil
}

func (p *provider6) MoveResourceState(context.Context, *tfplugin6.MoveResourceState_Request) (*tfplugin6.MoveResourceState_Response, error) {
	panic("Not Implemented")
}

func (p *provider6) ReadDataSource(ctx context.Context, req *tfplugin6.ReadDataSource_Request) (*tfplugin6.ReadDataSource_Response, error) {
	resp := &tfplugin6.ReadDataSource_Response{}
	ty := p.schema.DataSources[req.TypeName].Block.ImpliedType()

	configVal, err := decodeDynamicValue6(req.Config, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	metaTy := p.schema.ProviderMeta.Block.ImpliedType()
	metaVal, err := decodeDynamicValue6(req.ProviderMeta, metaTy)
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

	resp.State, err = encodeDynamicValue6(readResp.State, ty)
	if err != nil {
		resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, err)
		return resp, nil
	}

	return resp, nil
}

// OpenEphemeralResource implements tfplugin6.ProviderServer.
func (p *provider6) OpenEphemeralResource(ctx context.Context, req *tfplugin6.OpenEphemeralResource_Request) (*tfplugin6.OpenEphemeralResource_Response, error) {
	resp := &tfplugin6.OpenEphemeralResource_Response{}
	ty := p.schema.EphemeralResources[req.TypeName].Block.ImpliedType()

	configVal, err := decodeDynamicValue6(req.Config, ty)
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

	resp.Result, err = encodeDynamicValue6(openResp.Result, ty)
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

// RenewEphemeralResource implements tfplugin6.ProviderServer.
func (p *provider6) RenewEphemeralResource(ctx context.Context, req *tfplugin6.RenewEphemeralResource_Request) (*tfplugin6.RenewEphemeralResource_Response, error) {
	resp := &tfplugin6.RenewEphemeralResource_Response{}

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

// CloseEphemeralResource implements tfplugin6.ProviderServer.
func (p *provider6) CloseEphemeralResource(ctx context.Context, req *tfplugin6.CloseEphemeralResource_Request) (*tfplugin6.CloseEphemeralResource_Response, error) {
	resp := &tfplugin6.CloseEphemeralResource_Response{}

	renewResp := p.provider.CloseEphemeralResource(ctx, providers.CloseEphemeralResourceRequest{
		TypeName: req.TypeName,
		Private:  req.Private,
	})
	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, renewResp.Diagnostics)
	return resp, nil
}

func (p *provider6) StopProvider(ctx context.Context, _ *tfplugin6.StopProvider_Request) (*tfplugin6.StopProvider_Response, error) {
	resp := &tfplugin6.StopProvider_Response{}
	err := p.provider.Stop(ctx)
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

func (p *provider6) GetFunctions(context.Context, *tfplugin6.GetFunctions_Request) (*tfplugin6.GetFunctions_Response, error) {
	panic("Not Implemented")
}

func (p *provider6) CallFunction(context.Context, *tfplugin6.CallFunction_Request) (*tfplugin6.CallFunction_Response, error) {
	panic("Not Implemented")
}

// GetResourceIdentitySchemas implements tfplugin6.ProviderServer.
func (p *provider6) GetResourceIdentitySchemas(context.Context, *tfplugin6.GetResourceIdentitySchemas_Request) (*tfplugin6.GetResourceIdentitySchemas_Response, error) {
	panic("unimplemented")
}

// UpgradeResourceIdentity implements tfplugin6.ProviderServer.
func (p *provider6) UpgradeResourceIdentity(context.Context, *tfplugin6.UpgradeResourceIdentity_Request) (*tfplugin6.UpgradeResourceIdentity_Response, error) {
	panic("unimplemented")
}

func (p *provider6) GetMetadata(context.Context, *tfplugin6.GetMetadata_Request) (*tfplugin6.GetMetadata_Response, error) {
	panic("Not Implemented")
}

// decode a DynamicValue from either the JSON or MsgPack encoding.
func decodeDynamicValue6(v *tfplugin6.DynamicValue, ty cty.Type) (cty.Value, error) {
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
func encodeDynamicValue6(v cty.Value, ty cty.Type) (*tfplugin6.DynamicValue, error) {
	mp, err := msgpack.Marshal(v, ty)
	return &tfplugin6.DynamicValue{
		Msgpack: mp,
	}, err
}
