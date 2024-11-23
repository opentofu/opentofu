// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugin

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/zclconf/go-cty/cty"

	plugin "github.com/hashicorp/go-plugin"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"github.com/zclconf/go-cty/cty/msgpack"
	"google.golang.org/grpc"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/plugin/convert"
	"github.com/opentofu/opentofu/internal/providers"
	proto "github.com/opentofu/opentofu/internal/tfplugin5"
)

var logger = logging.HCLogger()

// GRPCProviderPlugin implements plugin.GRPCPlugin for the go-plugin package.
type GRPCProviderPlugin struct {
	plugin.Plugin
	GRPCProvider func() proto.ProviderServer
}

func (p *GRPCProviderPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &GRPCProvider{
		client: proto.NewProviderClient(c),
		ctx:    ctx,
	}, nil
}

func (p *GRPCProviderPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterProviderServer(s, p.GRPCProvider())
	return nil
}

// GRPCProvider handles the client, or core side of the plugin rpc connection.
// The GRPCProvider methods are mostly a translation layer between the
// tofu providers types and the grpc proto types, directly converting
// between the two.
type GRPCProvider struct {
	// PluginClient provides a reference to the plugin.Client which controls the plugin process.
	// This allows the GRPCProvider a way to shutdown the plugin process.
	PluginClient *plugin.Client

	// TestServer contains a grpc.Server to close when the GRPCProvider is being
	// used in an end to end test of a provider.
	TestServer *grpc.Server

	// Addr uniquely identifies the type of provider.
	// Normally executed providers will have this set during initialization,
	// but it may not always be available for alternative execute modes.
	Addr addrs.Provider

	// Proto client use to make the grpc service calls.
	client proto.ProviderClient

	// this context is created by the plugin package, and is canceled when the
	// plugin process ends.
	ctx context.Context

	mu sync.Mutex
	// schema stores the schema for this provider. This is used to properly
	// serialize the requests for schemas.
	schema providers.GetProviderSchemaResponse
}

var _ providers.Interface = new(GRPCProvider)

func (p *GRPCProvider) GetProviderSchema() (resp providers.GetProviderSchemaResponse) {
	logger.Trace("GRPCProvider: GetProviderSchema")
	p.mu.Lock()
	defer p.mu.Unlock()

	// First, we check the global cache.
	// The cache could contain this schema if an instance of this provider has previously been started.
	if !p.Addr.IsZero() {
		// Even if the schema is cached, GetProviderSchemaOptional could be false. This would indicate that once instantiated,
		// this provider requires the get schema call to be made at least once, as it handles part of the provider's setup.
		// At this point, we don't know if this is the first call to a provider instance or not, so we don't use the result in that case.
		if schemaCached, ok := providers.SchemaCache.Get(p.Addr); ok && schemaCached.ServerCapabilities.GetProviderSchemaOptional {
			logger.Trace("GRPCProvider: GetProviderSchema: serving from global schema cache", "address", p.Addr)
			return schemaCached
		}
	}

	// If the local cache is non-zero, we know this instance has called
	// GetProviderSchema at least once, so has satisfied the possible requirement of `GetProviderSchemaOptional=false`.
	// This means that we can return early now using the locally cached schema, without making this call again.
	if p.schema.Provider.Block != nil {
		return p.schema
	}

	resp.ResourceTypes = make(map[string]providers.Schema)
	resp.DataSources = make(map[string]providers.Schema)
	resp.Functions = make(map[string]providers.FunctionSpec)

	// Some providers may generate quite large schemas, and the internal default
	// grpc response size limit is 4MB. 64MB should cover most any use case, and
	// if we get providers nearing that we may want to consider a finer-grained
	// API to fetch individual resource schemas.
	// Note: this option is marked as EXPERIMENTAL in the grpc API. We keep
	// this for compatibility, but recent providers all set the max message
	// size much higher on the server side, which is the supported method for
	// determining payload size.
	const maxRecvSize = 64 << 20
	protoResp, err := p.client.GetSchema(p.ctx, new(proto.GetProviderSchema_Request), grpc.MaxRecvMsgSizeCallOption{MaxRecvMsgSize: maxRecvSize})
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(grpcErr(err))
		return resp
	}

	resp.Diagnostics = resp.Diagnostics.Append(convert.ProtoToDiagnostics(protoResp.Diagnostics))

	if resp.Diagnostics.HasErrors() {
		return resp
	}

	if protoResp.Provider == nil {
		resp.Diagnostics = resp.Diagnostics.Append(errors.New("missing provider schema"))
		return resp
	}

	resp.Provider = convert.ProtoToProviderSchema(protoResp.Provider)
	if protoResp.ProviderMeta == nil {
		logger.Debug("No provider meta schema returned")
	} else {
		resp.ProviderMeta = convert.ProtoToProviderSchema(protoResp.ProviderMeta)
	}

	for name, res := range protoResp.ResourceSchemas {
		resp.ResourceTypes[name] = convert.ProtoToProviderSchema(res)
	}

	for name, data := range protoResp.DataSourceSchemas {
		resp.DataSources[name] = convert.ProtoToProviderSchema(data)
	}

	for name, fn := range protoResp.Functions {
		resp.Functions[name] = convert.ProtoToFunctionSpec(fn)
	}

	if protoResp.ServerCapabilities != nil {
		resp.ServerCapabilities.PlanDestroy = protoResp.ServerCapabilities.PlanDestroy
		resp.ServerCapabilities.GetProviderSchemaOptional = protoResp.ServerCapabilities.GetProviderSchemaOptional
	}

	// Set the global provider cache so that future calls to this provider can use the cached value.
	// Crucially, this doesn't look at GetProviderSchemaOptional, because the layers above could use this cache
	// *without* creating an instance of this provider. And if there is no instance,
	// then we don't need to set up anything (cause there is nothing to set up), so we need no call
	// to the providers GetSchema rpc.
	if !p.Addr.IsZero() {
		providers.SchemaCache.Set(p.Addr, resp)
	}

	// Always store this here in the client for providers that are not able to use GetProviderSchemaOptional.
	// Crucially, this indicates that we've made at least one call to GetProviderSchema to this instance of the provider,
	// which means in the future we'll be able to return using this cache
	// (because the possible setup contained in the GetProviderSchema call has happened).
	// If GetProviderSchemaOptional is true then this cache won't actually ever be used, because the calls to this method
	// will be satisfied by the global provider cache.
	p.schema = resp

	return resp
}

func (p *GRPCProvider) ValidateProviderConfig(r providers.ValidateProviderConfigRequest) (resp providers.ValidateProviderConfigResponse) {
	logger.Trace("GRPCProvider: ValidateProviderConfig")

	schema := p.GetProviderSchema()
	if schema.Diagnostics.HasErrors() {
		resp.Diagnostics = schema.Diagnostics
		return resp
	}

	ty := schema.Provider.Block.ImpliedType()

	mp, err := msgpack.Marshal(r.Config, ty)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}

	protoReq := &proto.PrepareProviderConfig_Request{
		Config: &proto.DynamicValue{Msgpack: mp},
	}

	protoResp, err := p.client.PrepareProviderConfig(p.ctx, protoReq)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(grpcErr(err))
		return resp
	}

	config, err := decodeDynamicValue(protoResp.PreparedConfig, ty)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}
	resp.PreparedConfig = config

	resp.Diagnostics = resp.Diagnostics.Append(convert.ProtoToDiagnostics(protoResp.Diagnostics))
	return resp
}

func (p *GRPCProvider) ValidateResourceConfig(r providers.ValidateResourceConfigRequest) (resp providers.ValidateResourceConfigResponse) {
	logger.Trace("GRPCProvider: ValidateResourceConfig")

	schema := p.GetProviderSchema()
	if schema.Diagnostics.HasErrors() {
		resp.Diagnostics = schema.Diagnostics
		return resp
	}

	resourceSchema, ok := schema.ResourceTypes[r.TypeName]
	if !ok {
		resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf("unknown resource type %q", r.TypeName))
		return resp
	}

	mp, err := msgpack.Marshal(r.Config, resourceSchema.Block.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}

	protoReq := &proto.ValidateResourceTypeConfig_Request{
		TypeName: r.TypeName,
		Config:   &proto.DynamicValue{Msgpack: mp},
	}

	protoResp, err := p.client.ValidateResourceTypeConfig(p.ctx, protoReq)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(grpcErr(err))
		return resp
	}

	resp.Diagnostics = resp.Diagnostics.Append(convert.ProtoToDiagnostics(protoResp.Diagnostics))
	return resp
}

func (p *GRPCProvider) ValidateDataResourceConfig(r providers.ValidateDataResourceConfigRequest) (resp providers.ValidateDataResourceConfigResponse) {
	logger.Trace("GRPCProvider: ValidateDataResourceConfig")

	schema := p.GetProviderSchema()
	if schema.Diagnostics.HasErrors() {
		resp.Diagnostics = schema.Diagnostics
		return resp
	}

	dataSchema, ok := schema.DataSources[r.TypeName]
	if !ok {
		resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf("unknown data source %q", r.TypeName))
		return resp
	}

	mp, err := msgpack.Marshal(r.Config, dataSchema.Block.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}

	protoReq := &proto.ValidateDataSourceConfig_Request{
		TypeName: r.TypeName,
		Config:   &proto.DynamicValue{Msgpack: mp},
	}

	protoResp, err := p.client.ValidateDataSourceConfig(p.ctx, protoReq)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(grpcErr(err))
		return resp
	}
	resp.Diagnostics = resp.Diagnostics.Append(convert.ProtoToDiagnostics(protoResp.Diagnostics))
	return resp
}

func (p *GRPCProvider) UpgradeResourceState(r providers.UpgradeResourceStateRequest) (resp providers.UpgradeResourceStateResponse) {
	logger.Trace("GRPCProvider: UpgradeResourceState")

	schema := p.GetProviderSchema()
	if schema.Diagnostics.HasErrors() {
		resp.Diagnostics = schema.Diagnostics
		return resp
	}

	resSchema, ok := schema.ResourceTypes[r.TypeName]
	if !ok {
		resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf("unknown resource type %q", r.TypeName))
		return resp
	}

	protoReq := &proto.UpgradeResourceState_Request{
		TypeName: r.TypeName,
		Version:  int64(r.Version),
		RawState: &proto.RawState{
			Json:    r.RawStateJSON,
			Flatmap: r.RawStateFlatmap,
		},
	}

	protoResp, err := p.client.UpgradeResourceState(p.ctx, protoReq)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(grpcErr(err))
		return resp
	}
	resp.Diagnostics = resp.Diagnostics.Append(convert.ProtoToDiagnostics(protoResp.Diagnostics))

	ty := resSchema.Block.ImpliedType()
	resp.UpgradedState = cty.NullVal(ty)
	if protoResp.UpgradedState == nil {
		return resp
	}

	state, err := decodeDynamicValue(protoResp.UpgradedState, ty)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}
	resp.UpgradedState = state

	return resp
}

func (p *GRPCProvider) ConfigureProvider(r providers.ConfigureProviderRequest) (resp providers.ConfigureProviderResponse) {
	logger.Trace("GRPCProvider: ConfigureProvider")

	schema := p.GetProviderSchema()
	if schema.Diagnostics.HasErrors() {
		resp.Diagnostics = schema.Diagnostics
		return resp
	}

	var mp []byte

	// we don't have anything to marshal if there's no config
	mp, err := msgpack.Marshal(r.Config, schema.Provider.Block.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}

	protoReq := &proto.Configure_Request{
		TerraformVersion: r.TerraformVersion,
		Config: &proto.DynamicValue{
			Msgpack: mp,
		},
	}

	protoResp, err := p.client.Configure(p.ctx, protoReq)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(grpcErr(err))
		return resp
	}
	resp.Diagnostics = resp.Diagnostics.Append(convert.ProtoToDiagnostics(protoResp.Diagnostics))
	return resp
}

func (p *GRPCProvider) Stop() error {
	logger.Trace("GRPCProvider: Stop")

	resp, err := p.client.Stop(p.ctx, new(proto.Stop_Request))
	if err != nil {
		return err
	}

	if resp.Error != "" {
		return errors.New(resp.Error)
	}
	return nil
}

func (p *GRPCProvider) ReadResource(r providers.ReadResourceRequest) (resp providers.ReadResourceResponse) {
	logger.Trace("GRPCProvider: ReadResource")

	schema := p.GetProviderSchema()
	if schema.Diagnostics.HasErrors() {
		resp.Diagnostics = schema.Diagnostics
		return resp
	}

	resSchema, ok := schema.ResourceTypes[r.TypeName]
	if !ok {
		resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf("unknown resource type " + r.TypeName))
		return resp
	}

	metaSchema := schema.ProviderMeta

	mp, err := msgpack.Marshal(r.PriorState, resSchema.Block.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}

	protoReq := &proto.ReadResource_Request{
		TypeName:     r.TypeName,
		CurrentState: &proto.DynamicValue{Msgpack: mp},
		Private:      r.Private,
	}

	if metaSchema.Block != nil {
		metaMP, err := msgpack.Marshal(r.ProviderMeta, metaSchema.Block.ImpliedType())
		if err != nil {
			resp.Diagnostics = resp.Diagnostics.Append(err)
			return resp
		}
		protoReq.ProviderMeta = &proto.DynamicValue{Msgpack: metaMP}
	}

	protoResp, err := p.client.ReadResource(p.ctx, protoReq)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(grpcErr(err))
		return resp
	}
	resp.Diagnostics = resp.Diagnostics.Append(convert.ProtoToDiagnostics(protoResp.Diagnostics))

	state, err := decodeDynamicValue(protoResp.NewState, resSchema.Block.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}
	resp.NewState = state
	resp.Private = protoResp.Private

	return resp
}

func (p *GRPCProvider) PlanResourceChange(r providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
	logger.Trace("GRPCProvider: PlanResourceChange")

	schema := p.GetProviderSchema()
	if schema.Diagnostics.HasErrors() {
		resp.Diagnostics = schema.Diagnostics
		return resp
	}

	resSchema, ok := schema.ResourceTypes[r.TypeName]
	if !ok {
		resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf("unknown resource type %q", r.TypeName))
		return resp
	}

	metaSchema := schema.ProviderMeta
	capabilities := schema.ServerCapabilities

	// If the provider doesn't support planning a destroy operation, we can
	// return immediately.
	if r.ProposedNewState.IsNull() && !capabilities.PlanDestroy {
		resp.PlannedState = r.ProposedNewState
		resp.PlannedPrivate = r.PriorPrivate
		return resp
	}

	priorMP, err := msgpack.Marshal(r.PriorState, resSchema.Block.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}

	configMP, err := msgpack.Marshal(r.Config, resSchema.Block.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}

	propMP, err := msgpack.Marshal(r.ProposedNewState, resSchema.Block.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}

	protoReq := &proto.PlanResourceChange_Request{
		TypeName:         r.TypeName,
		PriorState:       &proto.DynamicValue{Msgpack: priorMP},
		Config:           &proto.DynamicValue{Msgpack: configMP},
		ProposedNewState: &proto.DynamicValue{Msgpack: propMP},
		PriorPrivate:     r.PriorPrivate,
	}

	if metaSchema.Block != nil {
		metaMP, err := msgpack.Marshal(r.ProviderMeta, metaSchema.Block.ImpliedType())
		if err != nil {
			resp.Diagnostics = resp.Diagnostics.Append(err)
			return resp
		}
		protoReq.ProviderMeta = &proto.DynamicValue{Msgpack: metaMP}
	}

	protoResp, err := p.client.PlanResourceChange(p.ctx, protoReq)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(grpcErr(err))
		return resp
	}
	resp.Diagnostics = resp.Diagnostics.Append(convert.ProtoToDiagnostics(protoResp.Diagnostics))

	state, err := decodeDynamicValue(protoResp.PlannedState, resSchema.Block.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}
	resp.PlannedState = state

	for _, p := range protoResp.RequiresReplace {
		resp.RequiresReplace = append(resp.RequiresReplace, convert.AttributePathToPath(p))
	}

	resp.PlannedPrivate = protoResp.PlannedPrivate

	resp.LegacyTypeSystem = protoResp.LegacyTypeSystem

	return resp
}

func (p *GRPCProvider) ApplyResourceChange(r providers.ApplyResourceChangeRequest) (resp providers.ApplyResourceChangeResponse) {
	logger.Trace("GRPCProvider: ApplyResourceChange")

	schema := p.GetProviderSchema()
	if schema.Diagnostics.HasErrors() {
		resp.Diagnostics = schema.Diagnostics
		return resp
	}

	resSchema, ok := schema.ResourceTypes[r.TypeName]
	if !ok {
		resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf("unknown resource type %q", r.TypeName))
		return resp
	}

	metaSchema := schema.ProviderMeta

	priorMP, err := msgpack.Marshal(r.PriorState, resSchema.Block.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}
	plannedMP, err := msgpack.Marshal(r.PlannedState, resSchema.Block.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}
	configMP, err := msgpack.Marshal(r.Config, resSchema.Block.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}

	protoReq := &proto.ApplyResourceChange_Request{
		TypeName:       r.TypeName,
		PriorState:     &proto.DynamicValue{Msgpack: priorMP},
		PlannedState:   &proto.DynamicValue{Msgpack: plannedMP},
		Config:         &proto.DynamicValue{Msgpack: configMP},
		PlannedPrivate: r.PlannedPrivate,
	}

	if metaSchema.Block != nil {
		metaMP, err := msgpack.Marshal(r.ProviderMeta, metaSchema.Block.ImpliedType())
		if err != nil {
			resp.Diagnostics = resp.Diagnostics.Append(err)
			return resp
		}
		protoReq.ProviderMeta = &proto.DynamicValue{Msgpack: metaMP}
	}

	protoResp, err := p.client.ApplyResourceChange(p.ctx, protoReq)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(grpcErr(err))
		return resp
	}
	resp.Diagnostics = resp.Diagnostics.Append(convert.ProtoToDiagnostics(protoResp.Diagnostics))

	resp.Private = protoResp.Private

	state, err := decodeDynamicValue(protoResp.NewState, resSchema.Block.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}
	resp.NewState = state

	resp.LegacyTypeSystem = protoResp.LegacyTypeSystem

	return resp
}

func (p *GRPCProvider) ImportResourceState(r providers.ImportResourceStateRequest) (resp providers.ImportResourceStateResponse) {
	logger.Trace("GRPCProvider: ImportResourceState")

	schema := p.GetProviderSchema()
	if schema.Diagnostics.HasErrors() {
		resp.Diagnostics = schema.Diagnostics
		return resp
	}

	protoReq := &proto.ImportResourceState_Request{
		TypeName: r.TypeName,
		Id:       r.ID,
	}

	protoResp, err := p.client.ImportResourceState(p.ctx, protoReq)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(grpcErr(err))
		return resp
	}
	resp.Diagnostics = resp.Diagnostics.Append(convert.ProtoToDiagnostics(protoResp.Diagnostics))

	for _, imported := range protoResp.ImportedResources {
		resource := providers.ImportedResource{
			TypeName: imported.TypeName,
			Private:  imported.Private,
		}

		resSchema, ok := schema.ResourceTypes[r.TypeName]
		if !ok {
			resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf("unknown resource type %q", r.TypeName))
			continue
		}

		state, err := decodeDynamicValue(imported.State, resSchema.Block.ImpliedType())
		if err != nil {
			resp.Diagnostics = resp.Diagnostics.Append(err)
			return resp
		}
		resource.State = state
		resp.ImportedResources = append(resp.ImportedResources, resource)
	}

	return resp
}

func (p *GRPCProvider) ReadDataSource(r providers.ReadDataSourceRequest) (resp providers.ReadDataSourceResponse) {
	logger.Trace("GRPCProvider: ReadDataSource")

	schema := p.GetProviderSchema()
	if schema.Diagnostics.HasErrors() {
		resp.Diagnostics = schema.Diagnostics
		return resp
	}

	dataSchema, ok := schema.DataSources[r.TypeName]
	if !ok {
		schema.Diagnostics = schema.Diagnostics.Append(fmt.Errorf("unknown data source %q", r.TypeName))
	}

	metaSchema := schema.ProviderMeta

	config, err := msgpack.Marshal(r.Config, dataSchema.Block.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}

	protoReq := &proto.ReadDataSource_Request{
		TypeName: r.TypeName,
		Config: &proto.DynamicValue{
			Msgpack: config,
		},
	}

	if metaSchema.Block != nil {
		metaMP, err := msgpack.Marshal(r.ProviderMeta, metaSchema.Block.ImpliedType())
		if err != nil {
			resp.Diagnostics = resp.Diagnostics.Append(err)
			return resp
		}
		protoReq.ProviderMeta = &proto.DynamicValue{Msgpack: metaMP}
	}

	protoResp, err := p.client.ReadDataSource(p.ctx, protoReq)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(grpcErr(err))
		return resp
	}
	resp.Diagnostics = resp.Diagnostics.Append(convert.ProtoToDiagnostics(protoResp.Diagnostics))

	state, err := decodeDynamicValue(protoResp.State, dataSchema.Block.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}
	resp.State = state

	return resp
}

func (p *GRPCProvider) GetFunctions() (resp providers.GetFunctionsResponse) {
	logger.Trace("GRPCProvider: GetFunctions")

	protoReq := &proto.GetFunctions_Request{}

	protoResp, err := p.client.GetFunctions(p.ctx, protoReq)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(grpcErr(err))
		return resp
	}
	resp.Diagnostics = resp.Diagnostics.Append(convert.ProtoToDiagnostics(protoResp.Diagnostics))
	resp.Functions = make(map[string]providers.FunctionSpec)

	for name, fn := range protoResp.Functions {
		resp.Functions[name] = convert.ProtoToFunctionSpec(fn)
	}

	return resp
}

func (p *GRPCProvider) CallFunction(r providers.CallFunctionRequest) (resp providers.CallFunctionResponse) {
	logger.Trace("GRPCProvider: CallFunction")

	schema := p.GetProviderSchema()
	if schema.Diagnostics.HasErrors() {
		// This should be unreachable
		resp.Error = schema.Diagnostics.Err()
		return resp
	}

	spec, ok := schema.Functions[r.Name]
	if !ok {
		funcs := p.GetFunctions()
		if funcs.Diagnostics.HasErrors() {
			// This should be unreachable
			resp.Error = funcs.Diagnostics.Err()
			return resp
		}
		spec, ok = funcs.Functions[r.Name]
		if !ok {
			// This should be unreachable
			resp.Error = fmt.Errorf("invalid CallFunctionRequest: function %s not defined in provider schema", r.Name)
			return resp
		}
	}

	protoReq := &proto.CallFunction_Request{
		Name:      r.Name,
		Arguments: make([]*proto.DynamicValue, len(r.Arguments)),
	}

	// Translate the arguments
	// As this is functionality is always sitting behind cty/function.Function, we skip some validation
	// checks of from the function and param spec.  We still include basic validation to prevent panics,
	// just in case there are bugs in cty.  See context_functions_test.go for explicit testing of argument
	// handling and short-circuiting.
	if len(r.Arguments) < len(spec.Parameters) {
		// This should be unreachable
		resp.Error = fmt.Errorf("invalid CallFunctionRequest: function %s expected %d parameters and got %d instead", r.Name, len(spec.Parameters), len(r.Arguments))
		return resp
	}

	for i, arg := range r.Arguments {
		var paramSpec providers.FunctionParameterSpec
		if i < len(spec.Parameters) {
			paramSpec = spec.Parameters[i]
		} else {
			// We are past the end of spec.Parameters, this is either variadic or an error
			if spec.VariadicParameter != nil {
				paramSpec = *spec.VariadicParameter
			} else {
				// This should be unreachable
				resp.Error = fmt.Errorf("invalid CallFunctionRequest: too many arguments passed to non-variadic function %s", r.Name)
				return resp
			}
		}

		if !paramSpec.AllowUnknownValues && !arg.IsWhollyKnown() {
			// Unlike the standard in cty, AllowUnknownValues == false does not just apply to
			// the root of the value (IsKnown) and instead also applies to values inside collections
			// and structures (IsWhollyKnown).
			// This is documented in the tfplugin proto file comments.
			//
			// The standard cty logic can be found in:
			// https://github.com/zclconf/go-cty/blob/ea922e7a95ba2be57897697117f318670e066d22/cty/function/function.go#L288-L290
			resp.Result = cty.UnknownVal(spec.Return)
			return resp
		}

		if arg.IsNull() {
			if paramSpec.AllowNullValue {
				continue
			} else {
				resp.Error = &providers.CallFunctionArgumentError{
					Text:             fmt.Sprintf("parameter %s is null, which is not allowed for function %s", paramSpec.Name, r.Name),
					FunctionArgument: i,
				}
			}

		}

		encodedArg, err := msgpack.Marshal(arg, paramSpec.Type)
		if err != nil {
			resp.Error = err
			return
		}

		protoReq.Arguments[i] = &proto.DynamicValue{
			Msgpack: encodedArg,
		}
	}

	protoResp, err := p.client.CallFunction(p.ctx, protoReq)
	if err != nil {
		resp.Error = err
		return
	}

	if protoResp.Error != nil {
		err := &providers.CallFunctionArgumentError{
			Text: protoResp.Error.Text,
		}
		if protoResp.Error.FunctionArgument != nil {
			err.FunctionArgument = int(*protoResp.Error.FunctionArgument)
		}
		resp.Error = err
		return
	}

	resp.Result, resp.Error = decodeDynamicValue(protoResp.Result, spec.Return)
	return
}

// closing the grpc connection is final, and tofu will call it at the end of every phase.
func (p *GRPCProvider) Close() error {
	logger.Trace("GRPCProvider: Close")

	// Make sure to stop the server if we're not running within go-plugin.
	if p.TestServer != nil {
		p.TestServer.Stop()
	}

	// Check this since it's not automatically inserted during plugin creation.
	// It's currently only inserted by the command package, because that is
	// where the factory is built and is the only point with access to the
	// plugin.Client.
	if p.PluginClient == nil {
		logger.Debug("provider has no plugin.Client")
		return nil
	}

	p.PluginClient.Kill()
	return nil
}

// Decode a DynamicValue from either the JSON or MsgPack encoding.
func decodeDynamicValue(v *proto.DynamicValue, ty cty.Type) (cty.Value, error) {
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
