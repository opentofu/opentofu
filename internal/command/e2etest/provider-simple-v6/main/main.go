// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	simple "github.com/opentofu/opentofu/internal/command/e2etest/provider-simple-v6"
	"github.com/opentofu/opentofu/internal/grpcwrap"
	"github.com/opentofu/opentofu/internal/plugin/plugintest"
	"github.com/opentofu/opentofu/internal/tfplugin6"
)

func main() {
	plugintest.Serve6(&plugintest.ServeOpts6{
		GRPCProviderFunc: func() tfplugin6.ProviderServer {
			return grpcwrap.Provider6(simple.Provider())
		},
	})
}
