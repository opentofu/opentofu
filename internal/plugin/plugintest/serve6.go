// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugintest

import (
	"log"

	goPlugin "github.com/hashicorp/go-plugin"
	"github.com/opentofu/opentofu/internal/plugin6"
	proto "github.com/opentofu/opentofu/internal/tfplugin6"
)

type GRPCProviderFunc6 func() proto.ProviderServer

// ServeOpts6 are the configurations to serve a plugin with version 6.
type ServeOpts6 struct {
	// Wrapped versions of the above plugins will automatically shimmed and
	// added to the GRPC functions when possible.
	GRPCProviderFunc GRPCProviderFunc6
}

// Serve serves a plugin. This function never returns and should be the final
// function called in the main function of the plugin.
func Serve6(opts *ServeOpts6) {
	goPlugin.Serve(&goPlugin.ServeConfig{
		HandshakeConfig:  plugin6.Handshake,
		VersionedPlugins: pluginSet6(opts),
		GRPCServer:       goPlugin.DefaultGRPCServer,
	})
}

func pluginSet6(opts *ServeOpts6) map[int]goPlugin.PluginSet {
	plugins := map[int]goPlugin.PluginSet{}
	log.Printf("[TRACE] plugintest.Serve6(%#v)", opts)

	// add the new protocol versions if they're configured
	if opts.GRPCProviderFunc != nil {
		plugins[6] = goPlugin.PluginSet{}
		if opts.GRPCProviderFunc != nil {
			plugins[6]["provider"] = &plugin6.GRPCProviderPlugin{
				GRPCProvider: opts.GRPCProviderFunc,
			}
		}
	}
	return plugins
}
