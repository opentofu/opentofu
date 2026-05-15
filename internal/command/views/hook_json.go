// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"bufio"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/format"
	"github.com/opentofu/opentofu/internal/command/views/json"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tofu"
)

// How long to wait between sending heartbeat/progress messages
const heartbeatInterval = 10 * time.Second

func newJSONHook(view *JSONView) *jsonHook {
	return &jsonHook{
		view:      view,
		applying:  make(map[string]applyProgress),
		timeNow:   time.Now,
		timeAfter: time.After,
	}
}

type jsonHook struct {
	tofu.NilHook

	view *JSONView

	applyingLock sync.Mutex
	// Concurrent map of resource addresses to allow the sequence of pre-apply,
	// progress, and post-apply messages to share data about the resource
	applying map[string]applyProgress

	// Mockable functions for testing the progress timer goroutine
	timeNow   func() time.Time
	timeAfter func(time.Duration) <-chan time.Time
}

var _ tofu.Hook = (*jsonHook)(nil)

type applyProgress struct {
	addr   addrs.AbsResourceInstance
	action plans.Action
	start  time.Time

	// done is used for post-apply to stop the progress goroutine
	done chan struct{}

	// heartbeatDone is used to allow tests to safely wait for the progress
	// goroutine to finish
	heartbeatDone chan struct{}
}

func (h *jsonHook) PreApply(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value) (tofu.HookAction, error) {
	return h.preApply(addr, gen, action, priorState, plannedNewState, nil)
}

// preApply implements PreApply. elapsedChan is non-nil only in tests:
// each heartbeat sends an elapsed value on it, and it is closed when
// the progress goroutine exits.
func (h *jsonHook) preApply(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value, elapsedChan chan<- time.Duration) (tofu.HookAction, error) {
	if action != plans.NoOp {
		idKey, idValue := format.ObjectValueIDOrName(priorState)
		h.view.Hook(json.NewApplyStart(addr, action, idKey, idValue))
	}

	progress := applyProgress{
		addr:          addr,
		action:        action,
		start:         h.timeNow().Round(time.Second),
		done:          make(chan struct{}),
		heartbeatDone: make(chan struct{}),
	}
	h.applyingLock.Lock()
	h.applying[addr.String()] = progress
	h.applyingLock.Unlock()

	if action != plans.NoOp {
		go h.applyingHeartbeat(progress, elapsedChan)
	}
	return tofu.HookActionContinue, nil
}

// applyingHeartbeat emits a periodic apply_progress message until the
// resource finishes. See preApply for the elapsedChan contract.
func (h *jsonHook) applyingHeartbeat(progress applyProgress, elapsedChan chan<- time.Duration) {
	defer close(progress.heartbeatDone)
	if elapsedChan != nil {
		defer close(elapsedChan)
	}
	for {
		select {
		case <-progress.done:
			return
		case <-h.timeAfter(heartbeatInterval):
		}

		elapsed := h.timeNow().Round(time.Second).Sub(progress.start)
		h.view.Hook(json.NewApplyProgress(progress.addr, progress.action, elapsed))
		if elapsedChan != nil {
			elapsedChan <- elapsed
		}
	}
}

func (h *jsonHook) PostApply(addr addrs.AbsResourceInstance, gen states.Generation, newState cty.Value, err error) (tofu.HookAction, error) {
	key := addr.String()
	h.applyingLock.Lock()
	progress := h.applying[key]
	if progress.done != nil {
		close(progress.done)
	}
	delete(h.applying, key)
	h.applyingLock.Unlock()

	if progress.action == plans.NoOp {
		return tofu.HookActionContinue, nil
	}

	elapsed := h.timeNow().Round(time.Second).Sub(progress.start)

	if err != nil {
		// Errors are collected and displayed post-apply, so no need to
		// re-render them here. Instead just signal that this resource failed
		// to apply.
		h.view.Hook(json.NewApplyErrored(addr, progress.action, elapsed))
	} else {
		idKey, idValue := format.ObjectValueID(newState)
		h.view.Hook(json.NewApplyComplete(addr, progress.action, idKey, idValue, elapsed))
	}
	return tofu.HookActionContinue, nil
}

func (h *jsonHook) PreProvisionInstanceStep(addr addrs.AbsResourceInstance, typeName string) (tofu.HookAction, error) {
	h.view.Hook(json.NewProvisionStart(addr, typeName))
	return tofu.HookActionContinue, nil
}

func (h *jsonHook) PostProvisionInstanceStep(addr addrs.AbsResourceInstance, typeName string, err error) (tofu.HookAction, error) {
	if err != nil {
		// Errors are collected and displayed post-apply, so no need to
		// re-render them here. Instead just signal that this provisioner step
		// failed.
		h.view.Hook(json.NewProvisionErrored(addr, typeName))
	} else {
		h.view.Hook(json.NewProvisionComplete(addr, typeName))
	}
	return tofu.HookActionContinue, nil
}

func (h *jsonHook) ProvisionOutput(addr addrs.AbsResourceInstance, typeName string, msg string, configMarks cty.ValueMarks) {
	// If the config has sensitive marks and showSensitive is not enabled,
	// suppress the output.
	if _, hasSensitive := configMarks[marks.Sensitive]; hasSensitive && !h.view.view.showSensitive {
		msg = "(output suppressed due to sensitive value in config)"
	}

	s := bufio.NewScanner(strings.NewReader(msg))
	s.Split(scanLines)
	for s.Scan() {
		line := strings.TrimRightFunc(s.Text(), unicode.IsSpace)
		if line != "" {
			h.view.Hook(json.NewProvisionProgress(addr, typeName, line))
		}
	}
}

func (h *jsonHook) PreRefresh(addr addrs.AbsResourceInstance, gen states.Generation, priorState cty.Value) (tofu.HookAction, error) {
	idKey, idValue := format.ObjectValueID(priorState)
	h.view.Hook(json.NewRefreshStart(addr, idKey, idValue))
	return tofu.HookActionContinue, nil
}

func (h *jsonHook) PostRefresh(addr addrs.AbsResourceInstance, gen states.Generation, priorState cty.Value, newState cty.Value) (tofu.HookAction, error) {
	idKey, idValue := format.ObjectValueID(newState)
	h.view.Hook(json.NewRefreshComplete(addr, idKey, idValue))
	return tofu.HookActionContinue, nil
}

func (h *jsonHook) PreOpen(addr addrs.AbsResourceInstance) (tofu.HookAction, error) {
	h.view.Hook(json.NewEphemeralStart(addr, "Opening..."))
	return tofu.HookActionContinue, nil
}

func (h *jsonHook) PostOpen(addr addrs.AbsResourceInstance, _ error) (tofu.HookAction, error) {
	h.view.Hook(json.NewEphemeralStop(addr, "Open complete"))
	return tofu.HookActionContinue, nil
}

func (h *jsonHook) PreRenew(addr addrs.AbsResourceInstance) (tofu.HookAction, error) {
	h.view.Hook(json.NewEphemeralStart(addr, "Renewing..."))
	return tofu.HookActionContinue, nil
}

func (h *jsonHook) PostRenew(addr addrs.AbsResourceInstance, _ error) (tofu.HookAction, error) {
	h.view.Hook(json.NewEphemeralStop(addr, "Renew complete"))
	return tofu.HookActionContinue, nil
}

func (h *jsonHook) PreClose(addr addrs.AbsResourceInstance) (tofu.HookAction, error) {
	h.view.Hook(json.NewEphemeralStart(addr, "Closing..."))
	return tofu.HookActionContinue, nil
}

func (h *jsonHook) PostClose(addr addrs.AbsResourceInstance, _ error) (tofu.HookAction, error) {
	h.view.Hook(json.NewEphemeralStop(addr, "Close complete"))
	return tofu.HookActionContinue, nil
}
