// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package graph

import (
	"errors"
	"sync/atomic"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tofu/hooks"
)

// stopHook is a private Hook implementation that OpenTofu uses to
// signal when to stop or cancel actions.
type stopHook struct {
	stop uint32
}

var _ hooks.Hook = (*stopHook)(nil)

func (h *stopHook) PreApply(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostApply(addr addrs.AbsResourceInstance, gen states.Generation, newState cty.Value, err error) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PreDiff(addr addrs.AbsResourceInstance, gen states.Generation, priorState, proposedNewState cty.Value) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostDiff(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PreProvisionInstance(addr addrs.AbsResourceInstance, state cty.Value) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostProvisionInstance(addr addrs.AbsResourceInstance, state cty.Value) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PreProvisionInstanceStep(addr addrs.AbsResourceInstance, typeName string) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostProvisionInstanceStep(addr addrs.AbsResourceInstance, typeName string, err error) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) ProvisionOutput(addr addrs.AbsResourceInstance, typeName string, line string) {
}

func (h *stopHook) PreRefresh(addr addrs.AbsResourceInstance, gen states.Generation, priorState cty.Value) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostRefresh(addr addrs.AbsResourceInstance, gen states.Generation, priorState cty.Value, newState cty.Value) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PreImportState(addr addrs.AbsResourceInstance, importID string) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostImportState(addr addrs.AbsResourceInstance, imported []providers.ImportedResource) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PrePlanImport(addr addrs.AbsResourceInstance, importID string) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostPlanImport(addr addrs.AbsResourceInstance, imported []providers.ImportedResource) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PreApplyImport(addr addrs.AbsResourceInstance, importing plans.ImportingSrc) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostApplyImport(addr addrs.AbsResourceInstance, importing plans.ImportingSrc) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PreApplyForget(_ addrs.AbsResourceInstance) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostApplyForget(_ addrs.AbsResourceInstance) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) Deferred(_ addrs.AbsResourceInstance, _ string) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PreOpen(_ addrs.AbsResourceInstance) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostOpen(_ addrs.AbsResourceInstance, _ error) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PreRenew(_ addrs.AbsResourceInstance) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostRenew(_ addrs.AbsResourceInstance, _ error) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PreClose(_ addrs.AbsResourceInstance) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostClose(_ addrs.AbsResourceInstance, _ error) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) Stopping() {}

func (h *stopHook) PostStateUpdate(func(*states.SyncState)) (hooks.HookAction, error) {
	return h.hook()
}

func (h *stopHook) hook() (hooks.HookAction, error) {
	if h.Stopped() {
		return hooks.HookActionHalt, errors.New("execution halted")
	}

	return hooks.HookActionContinue, nil
}

// reset should be called within the lock context
func (h *stopHook) Reset() {
	atomic.StoreUint32(&h.stop, 0)
}

func (h *stopHook) Stop() {
	atomic.StoreUint32(&h.stop, 1)
}

func (h *stopHook) Stopped() bool {
	return atomic.LoadUint32(&h.stop) == 1
}
