// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	localexec "github.com/terramate-io/opentofulib/internal/builtin/provisioners/local-exec"
	"github.com/terramate-io/opentofulib/internal/grpcwrap"
	"github.com/terramate-io/opentofulib/internal/plugin"
	"github.com/terramate-io/opentofulib/internal/tfplugin5"
)

func main() {
	// Provide a binary version of the internal terraform provider for testing
	plugin.Serve(&plugin.ServeOpts{
		GRPCProvisionerFunc: func() tfplugin5.ProvisionerServer {
			return grpcwrap.Provisioner(localexec.New())
		},
	})
}
