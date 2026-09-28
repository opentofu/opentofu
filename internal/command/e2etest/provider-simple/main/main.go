// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	simple "github.com/opentofu/opentofu/internal/command/e2etest/provider-simple"
	"github.com/opentofu/opentofu/internal/grpcwrap"
	"github.com/opentofu/opentofu/internal/plugin/plugintest"
	"github.com/opentofu/opentofu/internal/tfplugin5"
)

func main() {
	plugintest.Serve5(&plugintest.ServeOpts5{
		GRPCProviderFunc: func() tfplugin5.ProviderServer {
			return grpcwrap.Provider(simple.Provider())
		},
	})
}
