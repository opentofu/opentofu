// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"errors"
	"sync/atomic"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
)

// stopHook is a private Hook implementation that OpenTofu uses to
// signal when to stop or cancel actions.
type stopHook struct {
	stop uint32
}

var _ Hook = (*stopHook)(nil)

func (h *stopHook) PreApply(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostApply(addr addrs.AbsResourceInstance, gen states.Generation, newState cty.Value, err error) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) PreDiff(addr addrs.AbsResourceInstance, gen states.Generation, priorState, proposedNewState cty.Value) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostDiff(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) PreProvisionInstance(addr addrs.AbsResourceInstance, state cty.Value) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostProvisionInstance(addr addrs.AbsResourceInstance, state cty.Value) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) PreProvisionInstanceStep(addr addrs.AbsResourceInstance, typeName string) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostProvisionInstanceStep(addr addrs.AbsResourceInstance, typeName string, err error) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) ProvisionOutput(addr addrs.AbsResourceInstance, typeName string, line string) {
}

func (h *stopHook) PreRefresh(addr addrs.AbsResourceInstance, gen states.Generation, priorState cty.Value) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostRefresh(addr addrs.AbsResourceInstance, gen states.Generation, priorState cty.Value, newState cty.Value) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) PreImportState(addr addrs.AbsResourceInstance, importID string) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostImportState(addr addrs.AbsResourceInstance, imported []providers.ImportedResource) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) PrePlanImport(addr addrs.AbsResourceInstance, importID string) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostPlanImport(addr addrs.AbsResourceInstance, imported []providers.ImportedResource) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) PreApplyImport(addr addrs.AbsResourceInstance, importing plans.ImportingSrc) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostApplyImport(addr addrs.AbsResourceInstance, importing plans.ImportingSrc) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) PreApplyForget(_ addrs.AbsResourceInstance) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) PostApplyForget(_ addrs.AbsResourceInstance) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) Stopping() {}

func (h *stopHook) PostStateUpdate(new *states.State) (HookAction, error) {
	return h.hook()
}

func (h *stopHook) hook() (HookAction, error) {
	if h.Stopped() {
		return HookActionHalt, errors.New("execution halted")
	}

	return HookActionContinue, nil
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
