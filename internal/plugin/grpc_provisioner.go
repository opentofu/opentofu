// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugin

import (
	"context"
	"errors"
	"io"
	"sync"

	proto "github.com/apparentlymart/opentofu-providers/tofuprovider/grpc/tfplugin5"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/plugin/convert"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/msgpack"
	"go.rpcplugin.org/rpcplugin"
	"google.golang.org/grpc"
)

// provisioners.Interface grpc implementation
type GRPCProvisioner struct {
	// PluginClient provides a reference to the rpcplugin.Plugin which controls
	// the plugin process. This allows the GRPCProvisioner a way to shutdown
	// the plugin process.
	PluginClient *rpcplugin.Plugin

	client proto.ProvisionerClient
	ctx    context.Context

	mu sync.Mutex
	// Cache the schema since we need it for serialization in each method call.
	schema *configschema.Block
}

func NewGRPCProvisioner(ctx context.Context, conn *grpc.ClientConn) provisioners.Interface {
	return &GRPCProvisioner{
		client: proto.NewProvisionerClient(conn),
		ctx:    ctx,
	}
}

func (p *GRPCProvisioner) GetSchema() (resp provisioners.GetSchemaResponse) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.schema != nil {
		return provisioners.GetSchemaResponse{
			Provisioner: p.schema,
		}
	}

	protoResp, err := p.client.GetSchema(p.ctx, new(proto.GetProvisionerSchema_Request))
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(grpcErr(err))
		return resp
	}
	resp.Diagnostics = resp.Diagnostics.Append(convert.ProtoToDiagnostics(protoResp.Diagnostics))

	if protoResp.Provisioner == nil {
		resp.Diagnostics = resp.Diagnostics.Append(errors.New("missing provisioner schema"))
		return resp
	}

	resp.Provisioner = convert.ProtoToConfigSchema(protoResp.Provisioner.Block)

	p.schema = resp.Provisioner

	return resp
}

func (p *GRPCProvisioner) ValidateProvisionerConfig(r provisioners.ValidateProvisionerConfigRequest) (resp provisioners.ValidateProvisionerConfigResponse) {
	schema := p.GetSchema()
	if schema.Diagnostics.HasErrors() {
		resp.Diagnostics = resp.Diagnostics.Append(schema.Diagnostics)
		return resp
	}

	mp, err := msgpack.Marshal(r.Config, schema.Provisioner.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}

	protoReq := &proto.ValidateProvisionerConfig_Request{
		Config: &proto.DynamicValue{Msgpack: mp},
	}
	protoResp, err := p.client.ValidateProvisionerConfig(p.ctx, protoReq)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(grpcErr(err))
		return resp
	}
	resp.Diagnostics = resp.Diagnostics.Append(convert.ProtoToDiagnostics(protoResp.Diagnostics))
	return resp
}

func (p *GRPCProvisioner) ProvisionResource(r provisioners.ProvisionResourceRequest) (resp provisioners.ProvisionResourceResponse) {
	schema := p.GetSchema()
	if schema.Diagnostics.HasErrors() {
		resp.Diagnostics = resp.Diagnostics.Append(schema.Diagnostics)
		return resp
	}

	mp, err := msgpack.Marshal(r.Config, schema.Provisioner.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}

	// connection is always assumed to be a simple string map
	connMP, err := msgpack.Marshal(r.Connection, cty.Map(cty.String))
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}

	protoReq := &proto.ProvisionResource_Request{
		Config:     &proto.DynamicValue{Msgpack: mp},
		Connection: &proto.DynamicValue{Msgpack: connMP},
	}

	outputClient, err := p.client.ProvisionResource(p.ctx, protoReq)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(grpcErr(err))
		return resp
	}

	for {
		rcv, err := outputClient.Recv()
		if rcv != nil {
			r.UIOutput.Output(rcv.Output)
		}
		if err != nil {
			if err != io.EOF {
				resp.Diagnostics = resp.Diagnostics.Append(err)
			}
			break
		}

		if len(rcv.Diagnostics) > 0 {
			resp.Diagnostics = resp.Diagnostics.Append(convert.ProtoToDiagnostics(rcv.Diagnostics))
			break
		}
	}

	return resp
}

func (p *GRPCProvisioner) Stop() error {
	protoResp, err := p.client.Stop(p.ctx, &proto.Stop_Request{})
	if err != nil {
		return err
	}
	if protoResp.Error != "" {
		return errors.New(protoResp.Error)
	}
	return nil
}

func (p *GRPCProvisioner) Close() error {
	// check this since it's not automatically inserted during plugin creation
	if p.PluginClient == nil {
		logger.Debug("provisioner has no plugin.Client")
		return nil
	}

	return p.PluginClient.Close()
}
