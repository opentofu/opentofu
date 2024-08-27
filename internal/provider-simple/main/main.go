// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"github.com/terramate-io/opentofulib/internal/grpcwrap"
	"github.com/terramate-io/opentofulib/internal/plugin"
	simple "github.com/terramate-io/opentofulib/internal/provider-simple"
	"github.com/terramate-io/opentofulib/internal/tfplugin5"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		GRPCProviderFunc: func() tfplugin5.ProviderServer {
			return grpcwrap.Provider(simple.Provider())
		},
	})
}
