// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"github.com/opentofu/opentofu/internal/addrs"
)

// updateStateHook calls the PostStateUpdate hook with the current state.
func updateStateHook(addr addrs.AbsResourceInstance, ctx EvalContext, provider addrs.AbsProviderConfig) error {
	// Call the hook
	return ctx.Hook(func(h Hook) (HookAction, error) {
		return h.PostStateUpdate(addr, ctx.State().ResourceInstance(addr), provider)
	})
}
