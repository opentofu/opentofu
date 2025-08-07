// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"github.com/opentofu/opentofu/internal/states"
)

// updateState calls the PostStateUpdate hook with the state modification function and applies the change to the "live" state object
func updateState(ctx EvalContext, fn func(*states.SyncState)) error {
	// Since the EvalContext.State() is not in sync with the StateMgr hold one, we want to update both in the same call.
	fn(ctx.State())

	// Call the hook
	return ctx.Hook(func(h Hook) (HookAction, error) {
		return h.PostStateUpdate(fn)
	})
}
