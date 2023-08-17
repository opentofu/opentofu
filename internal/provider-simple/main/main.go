// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"github.com/placeholderplaceholderplaceholder/opentf/internal/grpcwrap"
	"github.com/placeholderplaceholderplaceholder/opentf/internal/plugin"
	simple "github.com/placeholderplaceholderplaceholder/opentf/internal/provider-simple"
	"github.com/placeholderplaceholderplaceholder/opentf/internal/tfplugin5"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		GRPCProviderFunc: func() tfplugin5.ProviderServer {
			return grpcwrap.Provider(simple.Provider())
		},
	})
}
