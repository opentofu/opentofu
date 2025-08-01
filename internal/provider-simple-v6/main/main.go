// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"github.com/apparentlymart/opentofu-providers/tofuprovider/grpc/tfplugin6"

	"github.com/opentofu/opentofu/internal/grpcwrap"
	plugin "github.com/opentofu/opentofu/internal/plugin6"
	simple "github.com/opentofu/opentofu/internal/provider-simple-v6"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		GRPCProviderFunc: func() tfplugin6.ProviderServer {
			return grpcwrap.Provider6(simple.Provider())
		},
	})
}
