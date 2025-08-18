// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import "github.com/opentofu/opentofu/internal/states/statekeys"

// updateStateHook calls the PostStateUpdate hook with the current state.
func updateStateHook(ctx EvalContext, key statekeys.Key) error {
	// In principle we could grab the lock here just long enough to take a
	// deep copy and then pass that to our hooks below, but we'll instead
	// hold the hook for the duration to avoid the potential confusing
	// situation of us racing to call PostStateUpdate concurrently with
	// different state snapshots.
	stateSync := ctx.State()
	state := stateSync.Lock().DeepCopy()
	defer stateSync.Unlock()

	// Call the hook
	if key != nil {
		err := ctx.Hook(func(h Hook) (HookAction, error) {
			return HookActionContinue, h.StateValueChanged(key, state)
		})
		if err != nil {
			return err
		}
	}
	err := ctx.Hook(func(h Hook) (HookAction, error) {
		return h.PostStateUpdate(state)
	})
	return err
}
