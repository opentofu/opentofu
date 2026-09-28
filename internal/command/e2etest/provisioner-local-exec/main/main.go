// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	localexec "github.com/opentofu/opentofu/internal/builtin/provisioners/local-exec"
	"github.com/opentofu/opentofu/internal/grpcwrap"
	"github.com/opentofu/opentofu/internal/plugin/plugintest"
	"github.com/opentofu/opentofu/internal/tfplugin5"
)

func main() {
	// Provide a binary version of the internal terraform provider for testing
	plugintest.Serve5(&plugintest.ServeOpts5{
		GRPCProvisionerFunc: func() tfplugin5.ProvisionerServer {
			return grpcwrap.Provisioner(localexec.New())
		},
	})
}
