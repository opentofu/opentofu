// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
)

// HookAction is an enum of actions that can be taken as a result of a hook
// callback. This allows you to modify the behavior of OpenTofu at runtime.
type HookAction byte

const (
	// HookActionContinue continues with processing as usual.
	HookActionContinue HookAction = iota

	// HookActionHalt halts immediately: no more hooks are processed
	// and the action that OpenTofu was about to take is cancelled.
	HookActionHalt
)

// Hook is the interface that must be implemented to hook into various
// parts of OpenTofu, allowing you to inspect or change behavior at runtime.
//
// There are MANY hook points into OpenTofu. If you only want to implement
// some hook points, but not all (which is the likely case), then embed the
// NilHook into your struct, which implements all of the interface but does
// nothing. Then, override only the functions you want to implement.
type Hook interface {
	// PreApply and PostApply are called before and after an action for a
	// single instance is applied. The error argument in PostApply is the
	// error, if any, that was returned from the provider Apply call itself.
	PreApply(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value) (HookAction, error)
	PostApply(addr addrs.AbsResourceInstance, gen states.Generation, newState cty.Value, err error) (HookAction, error)

	// PreDiff and PostDiff are called before and after a provider is given
	// the opportunity to customize the proposed new state to produce the
	// planned new state.
	PreDiff(addr addrs.AbsResourceInstance, gen states.Generation, priorState, proposedNewState cty.Value) (HookAction, error)
	PostDiff(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value) (HookAction, error)

	// The provisioning hooks signal both the overall start end end of
	// provisioning for a particular instance and of each of the individual
	// configured provisioners for each instance. The sequence of these
	// for a given instance might look something like this:
	//
	//          PreProvisionInstance(aws_instance.foo[1], ...)
	//      PreProvisionInstanceStep(aws_instance.foo[1], "file")
	//     PostProvisionInstanceStep(aws_instance.foo[1], "file", nil)
	//      PreProvisionInstanceStep(aws_instance.foo[1], "remote-exec")
	//               ProvisionOutput(aws_instance.foo[1], "remote-exec", "Installing foo...")
	//               ProvisionOutput(aws_instance.foo[1], "remote-exec", "Configuring bar...")
	//     PostProvisionInstanceStep(aws_instance.foo[1], "remote-exec", nil)
	//         PostProvisionInstance(aws_instance.foo[1], ...)
	//
	// ProvisionOutput is called with output sent back by the provisioners.
	// This will be called multiple times as output comes in, with each call
	// representing one line of output. It cannot control whether the
	// provisioner continues running.
	PreProvisionInstance(addr addrs.AbsResourceInstance, state cty.Value) (HookAction, error)
	PostProvisionInstance(addr addrs.AbsResourceInstance, state cty.Value) (HookAction, error)
	PreProvisionInstanceStep(addr addrs.AbsResourceInstance, typeName string) (HookAction, error)
	PostProvisionInstanceStep(addr addrs.AbsResourceInstance, typeName string, err error) (HookAction, error)
	ProvisionOutput(addr addrs.AbsResourceInstance, typeName string, line string)

	// PreRefresh and PostRefresh are called before and after a single
	// resource state is refreshed, respectively.
	PreRefresh(addr addrs.AbsResourceInstance, gen states.Generation, priorState cty.Value) (HookAction, error)
	PostRefresh(addr addrs.AbsResourceInstance, gen states.Generation, priorState cty.Value, newState cty.Value) (HookAction, error)

	// PreImportState and PostImportState are called before and after
	// (respectively) each state import operation for a given resource address when
	// using the legacy import command.
	PreImportState(addr addrs.AbsResourceInstance, importID string) (HookAction, error)
	PostImportState(addr addrs.AbsResourceInstance, imported []providers.ImportedResource) (HookAction, error)

	// PrePlanImport and PostPlanImport are called during a plan before and after planning to import
	// a new resource using the configuration-driven import workflow.
	PrePlanImport(addr addrs.AbsResourceInstance, importID string) (HookAction, error)
	PostPlanImport(addr addrs.AbsResourceInstance, imported []providers.ImportedResource) (HookAction, error)

	// PreApplyImport and PostApplyImport are called during an apply for each imported resource when
	// using the configuration-driven import workflow.
	PreApplyImport(addr addrs.AbsResourceInstance, importing plans.ImportingSrc) (HookAction, error)
	PostApplyImport(addr addrs.AbsResourceInstance, importing plans.ImportingSrc) (HookAction, error)

	// PreApplyForget and PostApplyForget are called during an apply for each forgotten resource.
	PreApplyForget(addr addrs.AbsResourceInstance) (HookAction, error)
	PostApplyForget(addr addrs.AbsResourceInstance) (HookAction, error)

	// Stopping is called if an external signal requests that OpenTofu
	// gracefully abort an operation in progress.
	//
	// This notification might suggest that the user wants OpenTofu to exit
	// ASAP and in that case it's possible that if OpenTofu runs for too much
	// longer then it'll get killed un-gracefully, and so this hook could be
	// an opportunity to persist any transient data that would be lost under
	// a subsequent kill signal. However, implementations must take care to do
	// so in a way that won't cause corruption if the process _is_ killed while
	// this hook is still running.
	//
	// This hook cannot control whether OpenTofu continues, because the
	// graceful shutdown process is typically already running by the time this
	// function is called.
	Stopping()

	// PostStateUpdate is called each time the state is updated. It receives
	// a deep copy of the state, which it may therefore access freely without
	// any need for locks to protect from concurrent writes from the caller.
	PostStateUpdate(new *states.State) (HookAction, error)
}

// NilHook is a Hook implementation that does nothing. It exists only to
// simplify implementing hooks. You can embed this into your Hook implementation
// and only implement the functions you are interested in.
type NilHook struct{}

var _ Hook = (*NilHook)(nil)

func (*NilHook) PreApply(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value) (HookAction, error) {
	return HookActionContinue, nil
}

func (*NilHook) PostApply(addr addrs.AbsResourceInstance, gen states.Generation, newState cty.Value, err error) (HookAction, error) {
	return HookActionContinue, nil
}

func (*NilHook) PreDiff(addr addrs.AbsResourceInstance, gen states.Generation, priorState, proposedNewState cty.Value) (HookAction, error) {
	return HookActionContinue, nil
}

func (*NilHook) PostDiff(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value) (HookAction, error) {
	return HookActionContinue, nil
}

func (*NilHook) PreProvisionInstance(addr addrs.AbsResourceInstance, state cty.Value) (HookAction, error) {
	return HookActionContinue, nil
}

func (*NilHook) PostProvisionInstance(addr addrs.AbsResourceInstance, state cty.Value) (HookAction, error) {
	return HookActionContinue, nil
}

func (*NilHook) PreProvisionInstanceStep(addr addrs.AbsResourceInstance, typeName string) (HookAction, error) {
	return HookActionContinue, nil
}

func (*NilHook) PostProvisionInstanceStep(addr addrs.AbsResourceInstance, typeName string, err error) (HookAction, error) {
	return HookActionContinue, nil
}

func (*NilHook) ProvisionOutput(addr addrs.AbsResourceInstance, typeName string, line string) {
}

func (*NilHook) PreRefresh(addr addrs.AbsResourceInstance, gen states.Generation, priorState cty.Value) (HookAction, error) {
	return HookActionContinue, nil
}

func (*NilHook) PostRefresh(addr addrs.AbsResourceInstance, gen states.Generation, priorState cty.Value, newState cty.Value) (HookAction, error) {
	return HookActionContinue, nil
}

func (*NilHook) PreImportState(addr addrs.AbsResourceInstance, importID string) (HookAction, error) {
	return HookActionContinue, nil
}

func (*NilHook) PostImportState(addr addrs.AbsResourceInstance, imported []providers.ImportedResource) (HookAction, error) {
	return HookActionContinue, nil
}

func (h *NilHook) PrePlanImport(addr addrs.AbsResourceInstance, importID string) (HookAction, error) {
	return HookActionContinue, nil
}

func (h *NilHook) PostPlanImport(addr addrs.AbsResourceInstance, imported []providers.ImportedResource) (HookAction, error) {
	return HookActionContinue, nil
}

func (h *NilHook) PreApplyImport(addr addrs.AbsResourceInstance, importing plans.ImportingSrc) (HookAction, error) {
	return HookActionContinue, nil
}

func (h *NilHook) PostApplyImport(addr addrs.AbsResourceInstance, importing plans.ImportingSrc) (HookAction, error) {
	return HookActionContinue, nil
}

func (h *NilHook) PreApplyForget(_ addrs.AbsResourceInstance) (HookAction, error) {
	return HookActionContinue, nil
}

func (h *NilHook) PostApplyForget(_ addrs.AbsResourceInstance) (HookAction, error) {
	return HookActionContinue, nil
}

func (*NilHook) Stopping() {
	// Does nothing at all by default
}

func (*NilHook) PostStateUpdate(new *states.State) (HookAction, error) {
	return HookActionContinue, nil
}
