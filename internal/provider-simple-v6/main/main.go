// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"github.com/placeholderplaceholderplaceholder/opentf/internal/grpcwrap"
	plugin "github.com/placeholderplaceholderplaceholder/opentf/internal/plugin6"
	simple "github.com/placeholderplaceholderplaceholder/opentf/internal/provider-simple-v6"
	"github.com/placeholderplaceholderplaceholder/opentf/internal/tfplugin6"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		GRPCProviderFunc: func() tfplugin6.ProviderServer {
			return grpcwrap.Provider6(simple.Provider())
		},
	})
}
