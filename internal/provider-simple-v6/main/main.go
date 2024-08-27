// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"github.com/terramate-io/opentofulib/internal/grpcwrap"
	plugin "github.com/terramate-io/opentofulib/internal/plugin6"
	simple "github.com/terramate-io/opentofulib/internal/provider-simple-v6"
	"github.com/terramate-io/opentofulib/internal/tfplugin6"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		GRPCProviderFunc: func() tfplugin6.ProviderServer {
			return grpcwrap.Provider6(simple.Provider())
		},
	})
}
