// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"sync"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tofu"
)

// countHook is a hook that counts the number of resources
// added, removed, changed during the course of an apply.
type countHook struct {
	Added     int
	Changed   int
	Removed   int
	Imported  int
	Forgotten int

	ToAdd          int
	ToChange       int
	ToRemove       int
	ToRemoveAndAdd int

	sync.Mutex
	pending map[string]plans.Action

	tofu.NilHook
}

var _ tofu.Hook = (*countHook)(nil)

func (h *countHook) Reset() {
	h.Lock()
	defer h.Unlock()

	h.pending = nil
	h.Added = 0
	h.Changed = 0
	h.Removed = 0
	h.Imported = 0
	h.Forgotten = 0
}

func (h *countHook) PreApply(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value) (tofu.HookAction, error) {
	h.Lock()
	defer h.Unlock()

	if h.pending == nil {
		h.pending = make(map[string]plans.Action)
	}

	h.pending[addr.String()] = action

	return tofu.HookActionContinue, nil
}

func (h *countHook) PostApply(addr addrs.AbsResourceInstance, gen states.Generation, newState cty.Value, err error) (tofu.HookAction, error) {
	h.Lock()
	defer h.Unlock()

	if h.pending != nil {
		pendingKey := addr.String()
		if action, ok := h.pending[pendingKey]; ok {
			delete(h.pending, pendingKey)

			if err == nil {
				switch action {
				case plans.CreateThenDelete, plans.DeleteThenCreate:
					h.Added++
					h.Removed++
				case plans.Create:
					h.Added++
				case plans.Delete:
					h.Removed++
				case plans.Update:
					h.Changed++

				}
			}
		}
	}

	return tofu.HookActionContinue, nil
}

func (h *countHook) PostDiff(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value) (tofu.HookAction, error) {
	h.Lock()
	defer h.Unlock()

	// We don't count anything for data resources and neither for the ephemeral ones.
	// TODO ephemeral - test this after the ephemeral resources are introduced entirely
	if addr.Resource.Resource.Mode == addrs.DataResourceMode || addr.Resource.Resource.Mode == addrs.EphemeralResourceMode {
		return tofu.HookActionContinue, nil
	}

	switch action {
	case plans.CreateThenDelete, plans.DeleteThenCreate:
		h.ToRemoveAndAdd += 1
	case plans.Create:
		h.ToAdd += 1
	case plans.Delete:
		h.ToRemove += 1
	case plans.Update:
		h.ToChange += 1
	}

	return tofu.HookActionContinue, nil
}

func (h *countHook) PostApplyImport(addr addrs.AbsResourceInstance, importing plans.ImportingSrc) (tofu.HookAction, error) {
	h.Lock()
	defer h.Unlock()

	h.Imported++
	return tofu.HookActionContinue, nil
}

func (h *countHook) PostApplyForget(_ addrs.AbsResourceInstance) (tofu.HookAction, error) {
	h.Lock()
	defer h.Unlock()

	h.Forgotten++
	return tofu.HookActionContinue, nil
}
