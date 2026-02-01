// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"sync"
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tofu/hooks"
)

func TestNilHook_impl(t *testing.T) {
	var _ hooks.Hook = new(hooks.NilHook)
}

// testHook is a Hook implementation that logs the calls it receives.
// It is intended for testing that core code is emitting the correct hooks
// for a given situation.
type testHook struct {
	mu    sync.Mutex
	Calls []*testHookCall
}

var _ hooks.Hook = (*testHook)(nil)

// testHookCall represents a single call in testHook.
// This hook just logs string names to make it easy to write "want" expressions
// in tests that can DeepEqual against the real calls.
type testHookCall struct {
	Action     string
	InstanceID string
}

func (h *testHook) PreApply(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PreApply", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PostApply(addr addrs.AbsResourceInstance, gen states.Generation, newState cty.Value, err error) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PostApply", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PreDiff(addr addrs.AbsResourceInstance, gen states.Generation, priorState, proposedNewState cty.Value) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PreDiff", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PostDiff(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PostDiff", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PreProvisionInstance(addr addrs.AbsResourceInstance, state cty.Value) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PreProvisionInstance", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PostProvisionInstance(addr addrs.AbsResourceInstance, state cty.Value) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PostProvisionInstance", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PreProvisionInstanceStep(addr addrs.AbsResourceInstance, typeName string) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PreProvisionInstanceStep", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PostProvisionInstanceStep(addr addrs.AbsResourceInstance, typeName string, err error) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PostProvisionInstanceStep", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) ProvisionOutput(addr addrs.AbsResourceInstance, typeName string, line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"ProvisionOutput", addr.String()})
}

func (h *testHook) PreRefresh(addr addrs.AbsResourceInstance, gen states.Generation, priorState cty.Value) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PreRefresh", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PostRefresh(addr addrs.AbsResourceInstance, gen states.Generation, priorState cty.Value, newState cty.Value) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PostRefresh", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PreImportState(addr addrs.AbsResourceInstance, importID string) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PreImportState", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PostImportState(addr addrs.AbsResourceInstance, imported []providers.ImportedResource) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PostImportState", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PrePlanImport(addr addrs.AbsResourceInstance, importID string) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PrePlanImport", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PostPlanImport(addr addrs.AbsResourceInstance, imported []providers.ImportedResource) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PostPlanImport", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PreApplyImport(addr addrs.AbsResourceInstance, importing plans.ImportingSrc) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PreApplyImport", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PostApplyImport(addr addrs.AbsResourceInstance, importing plans.ImportingSrc) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PostApplyImport", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PreApplyForget(addr addrs.AbsResourceInstance) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PreApplyForget", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PostApplyForget(addr addrs.AbsResourceInstance) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PostApplyForget", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) Deferred(addr addrs.AbsResourceInstance, reason string) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"Deferred", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PreOpen(addr addrs.AbsResourceInstance) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PreOpen", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PostOpen(addr addrs.AbsResourceInstance, _ error) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PostOpen", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PreRenew(addr addrs.AbsResourceInstance) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PreRenew", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PostRenew(addr addrs.AbsResourceInstance, _ error) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PostRenew", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PreClose(addr addrs.AbsResourceInstance) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PreClose", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) PostClose(addr addrs.AbsResourceInstance, _ error) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PostClose", addr.String()})
	return hooks.HookActionContinue, nil
}

func (h *testHook) Stopping() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"Stopping", ""})
}

func (h *testHook) PostStateUpdate(fn func(*states.SyncState)) (hooks.HookAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls = append(h.Calls, &testHookCall{"PostStateUpdate", ""})
	return hooks.HookActionContinue, nil
}
