// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugin

import (
	"net/rpc"

	"github.com/opentofu/opentofu/internal/tofu"
)

// UIOutput is an implementation of tofu.UIOutput that communicates
// over RPC.
type UIOutput struct {
	Client *rpc.Client
}

func (o *UIOutput) Output(v string) {
	o.Client.Call("Plugin.Output", v, new(interface{}))
}

// UIOutputServer is the RPC server for serving UIOutput.
type UIOutputServer struct {
	UIOutput tofu.UIOutput
}

func (s *UIOutputServer) Output(
	v string,
	reply *interface{}) error {
	s.UIOutput.Output(v)
	return nil
}
