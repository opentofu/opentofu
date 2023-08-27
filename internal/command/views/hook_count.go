// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"sync"

	"github.com/zclconf/go-cty/cty"

	"github.com/placeholderplaceholderplaceholder/opentf/internal/addrs"
	"github.com/placeholderplaceholderplaceholder/opentf/internal/plans"
	"github.com/placeholderplaceholderplaceholder/opentf/internal/states"
	"github.com/placeholderplaceholderplaceholder/opentf/internal/opentf"
)

// countHook is a hook that counts the number of resources
// added, removed, changed during the course of an apply.
type countHook struct {
	Added    int
	Changed  int
	Removed  int
	Imported int

	ToAdd          int
	ToChange       int
	ToRemove       int
	ToRemoveAndAdd int

	pending map[string]plans.Action

	sync.Mutex
	opentf.NilHook
}

var _ opentf.Hook = (*countHook)(nil)

func (h *countHook) Reset() {
	h.Lock()
	defer h.Unlock()

	h.pending = nil
	h.Added = 0
	h.Changed = 0
	h.Removed = 0
	h.Imported = 0
}

func (h *countHook) PreApply(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value) (opentf.HookAction, error) {
	h.Lock()
	defer h.Unlock()

	if h.pending == nil {
		h.pending = make(map[string]plans.Action)
	}

	h.pending[addr.String()] = action

	return opentf.HookActionContinue, nil
}

func (h *countHook) PostApply(addr addrs.AbsResourceInstance, gen states.Generation, newState cty.Value, err error) (opentf.HookAction, error) {
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

	return opentf.HookActionContinue, nil
}

func (h *countHook) PostDiff(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value) (opentf.HookAction, error) {
	h.Lock()
	defer h.Unlock()

	// We don't count anything for data resources
	if addr.Resource.Resource.Mode == addrs.DataResourceMode {
		return opentf.HookActionContinue, nil
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

	return opentf.HookActionContinue, nil
}

func (h *countHook) PostApplyImport(addr addrs.AbsResourceInstance, importing plans.ImportingSrc) (opentf.HookAction, error) {
	h.Lock()
	defer h.Unlock()

	h.Imported++
	return opentf.HookActionContinue, nil
}
