// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states"
)

// updateState calls the PostStateUpdate hook with the state modification function
func updateStateHook(evalCtx EvalContext, addr addrs.AbsResourceInstance) error {
	// Call the hook
	return evalCtx.Hook(func(h Hook) (HookAction, error) {
		return h.PostStateUpdate(func(s *states.SyncState) {
			provider := evalCtx.State().ResourceProvider(addr.ContainingResource())
			if provider == nil {
				// If there is no provider currently defined for the resource, it has been removed
				// See the documentation of ResourceProvider for more details
				s.RemoveResource(addr.ContainingResource())
			} else {
				s.SetResourceInstance(addr, evalCtx.State().ResourceInstance(addr), *provider)
			}
		})
	})
}
