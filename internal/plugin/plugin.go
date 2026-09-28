// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugin

import (
	"github.com/hashicorp/go-plugin"
	"github.com/opentofu/opentofu/internal/plugin6"
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
var Handshake = plugin.HandshakeConfig{
	// The ProtocolVersion is the version that must match between TF core
	// and TF plugins. This should be bumped whenever a change happens in
	// one or the other that makes it so that they can't safely communicate.
	// This could be adding a new interface value, it could be how
	// helper/schema computes diffs, etc.
	ProtocolVersion: DefaultProtocolVersion,

	// The magic cookie values should NEVER be changed.
	MagicCookieKey:   "TF_PLUGIN_MAGIC_COOKIE",
	MagicCookieValue: "d602bf8f470bc67ca7faa0386276bbdd4330efaf76d1a219cb4d6991ca9872b2",
}

// VersionedPlugins includes both protocol 5 and 6 because this is the function
// called in providerFactory (command/meta_providers.go) to set up the initial
// plugin client config.
var VersionedPlugins = map[int]plugin.PluginSet{
	5: {
		"provider":    &GRPCProviderPlugin{},
		"provisioner": &GRPCProvisionerPlugin{},
	},
	6: {
		"provider": &plugin6.GRPCProviderPlugin{},
	},
}
