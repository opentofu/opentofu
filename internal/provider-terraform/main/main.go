// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"github.com/placeholderplaceholderplaceholder/opentf/internal/builtin/providers/terraform"
	"github.com/placeholderplaceholderplaceholder/opentf/internal/grpcwrap"
	"github.com/placeholderplaceholderplaceholder/opentf/internal/plugin"
	"github.com/placeholderplaceholderplaceholder/opentf/internal/tfplugin5"
)

func main() {
	// Provide a binary version of the internal terraform provider for testing
	plugin.Serve(&plugin.ServeOpts{
		GRPCProviderFunc: func() tfplugin5.ProviderServer {
			return grpcwrap.Provider(terraform.NewProvider())
		},
	})
}
