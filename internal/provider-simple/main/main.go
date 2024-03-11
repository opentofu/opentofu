// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"github.com/opentofu/opentofu/internal/grpcwrap"
	"github.com/opentofu/opentofu/internal/plugin"
	simple "github.com/opentofu/opentofu/internal/provider-simple"
	"github.com/opentofu/opentofu/internal/tfplugin5"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		GRPCProviderFunc: func() tfplugin5.ProviderServer {
			return grpcwrap.Provider(simple.Provider())
		},
	})
}
