// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tofu/hooks"
)

// ProvisionerUIOutput is an implementation of UIOutput that calls a hook
// for the output so that the hooks can handle it.
type ProvisionerUIOutput struct {
	InstanceAddr    addrs.AbsResourceInstance
	ProvisionerType string
	Hooks           []hooks.Hook
}

func (o *ProvisionerUIOutput) Output(msg string) {
	for _, h := range o.Hooks {
		h.ProvisionOutput(o.InstanceAddr, o.ProvisionerType, msg)
	}
}
