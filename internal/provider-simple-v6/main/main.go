// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"github.com/opentffoundationgrpcwrap"
	plugin "github.com/opentffoundationplugin6"
	simple "github.com/opentffoundationprovider-simple-v6"
	"github.com/opentffoundationtfplugin6"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		GRPCProviderFunc: func() tfplugin6.ProviderServer {
			return grpcwrap.Provider6(simple.Provider())
		},
	})
}
