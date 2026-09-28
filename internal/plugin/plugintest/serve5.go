// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugintest

import (
	"log"

	goPlugin "github.com/hashicorp/go-plugin"
	"github.com/opentofu/opentofu/internal/plugin"
	proto "github.com/opentofu/opentofu/internal/tfplugin5"
)

type GRPCProviderFunc5 func() proto.ProviderServer
type GRPCProvisionerFunc5 func() proto.ProvisionerServer

// ServeOpts are the configurations to serve a plugin.
type ServeOpts5 struct {
	// Wrapped versions of the above plugins will automatically shimmed and
	// added to the GRPC functions when possible.
	GRPCProviderFunc    GRPCProviderFunc5
	GRPCProvisionerFunc GRPCProvisionerFunc5
}

// Serve serves a plugin. This function never returns and should be the final
// function called in the main function of the plugin.
func Serve5(opts *ServeOpts5) {
	goPlugin.Serve(&goPlugin.ServeConfig{
		HandshakeConfig:  plugin.Handshake,
		VersionedPlugins: pluginSet5(opts),
		GRPCServer:       goPlugin.DefaultGRPCServer,
	})
}

func pluginSet5(opts *ServeOpts5) map[int]goPlugin.PluginSet {
	plugins := map[int]goPlugin.PluginSet{}
	log.Printf("[TRACE] plugintest.Serve5(%#v)", opts)

	// add the new protocol versions if they're configured
	if opts.GRPCProviderFunc != nil || opts.GRPCProvisionerFunc != nil {
		plugins[5] = goPlugin.PluginSet{}
		if opts.GRPCProviderFunc != nil {
			plugins[5]["provider"] = &plugin.GRPCProviderPlugin{
				GRPCProvider: opts.GRPCProviderFunc,
			}
		}
		if opts.GRPCProvisionerFunc != nil {
			plugins[5]["provisioner"] = &plugin.GRPCProvisionerPlugin{
				GRPCProvisioner: opts.GRPCProvisionerFunc,
			}
		}
	}
	return plugins
}
