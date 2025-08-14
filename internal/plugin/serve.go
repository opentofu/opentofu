// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugin

import (
	"context"

	proto "github.com/apparentlymart/opentofu-providers/tofuprovider/grpc/tfplugin5"
	"go.rpcplugin.org/rpcplugin"
	"google.golang.org/grpc"
)

const (
	// The constants below are the names of the plugins that can be dispensed
	// from the plugin server.
	ProviderPluginName    = "provider"
	ProvisionerPluginName = "provisioner"

	// DefaultProtocolVersion is the protocol version assumed for legacy clients that don't specify
	// a particular version during their handshake. This is the version used when Terraform 0.10
	// and 0.11 launch plugins that were built with support for both versions 4 and 5, and must
	// stay unchanged at 4 until we intentionally build plugins that are not compatible with 0.10 and
	// 0.11.
	DefaultProtocolVersion = 4
)

// Handshake is the HandshakeConfig used to configure clients and servers.
var Handshake = rpcplugin.HandshakeConfig{
	// The magic cookie values should NEVER be changed.
	CookieKey:   "TF_PLUGIN_MAGIC_COOKIE",
	CookieValue: "d602bf8f470bc67ca7faa0386276bbdd4330efaf76d1a219cb4d6991ca9872b2",
}

type GRPCProviderFunc func() proto.ProviderServer
type GRPCProvisionerFunc func() proto.ProvisionerServer

// ServeOpts are the configurations to serve a plugin.
type ServeOpts struct {
	// Wrapped versions of the above plugins will automatically shimmed and
	// added to the GRPC functions when possible.
	GRPCProviderFunc    GRPCProviderFunc
	GRPCProvisionerFunc GRPCProvisionerFunc
}

// Serve serves a plugin. This function never returns and should be the final
// function called in the main function of the plugin.
func Serve(opts *ServeOpts) {
	err := rpcplugin.Serve(context.Background(), &rpcplugin.ServerConfig{
		Handshake:     Handshake,
		ProtoVersions: protoVersions(opts),
	})
	// This function is documented to never return, so if rpcplugin.Serve
	// returns we'll either panic (on error) or just block here forever
	// (on success).
	if err != nil {
		panic(err)
	}
	ch := make(chan struct{})
	<-ch // never returns, because nothing ever writes to this channel
}

func protoVersions(opts *ServeOpts) map[int]rpcplugin.ServerVersion {
	ret := make(map[int]rpcplugin.ServerVersion, 1)
	ret[5] = rpcplugin.ServerVersionFunc(func(s *grpc.Server) error {
		if opts.GRPCProviderFunc != nil {
			s.RegisterService(&proto.Provider_ServiceDesc, opts.GRPCProviderFunc())
		}
		if opts.GRPCProvisionerFunc != nil {
			s.RegisterService(&proto.Provisioner_ServiceDesc, opts.GRPCProvisionerFunc())
		}
		return nil
	})
	return ret
}
