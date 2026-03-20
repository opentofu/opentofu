// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Unlock interface {
	Diagnostics(diags tfdiags.Diagnostics)
	LockingDisabledForBackend()
	CannotUnlockByAnotherProcess()
	ForceUnlockCancelled()
	ForceUnlockSucceeded()
}

// NewUnlock returns an initialized Unlock implementation for the given ViewType.
func NewUnlock(args arguments.ViewOptions, view *View) Unlock {
	var ret Unlock
	switch args.ViewType {
	case arguments.ViewJSON:
		ret = &UnlockJSON{view: NewJSONView(view, nil)}
	case arguments.ViewHuman:
		ret = &UnlockHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewType))
	}

	if args.JSONInto != nil {
		ret = &UnlockMulti{ret, &UnlockJSON{view: NewJSONView(view, args.JSONInto)}}
	}
	return ret
}

type UnlockMulti []Unlock

var _ Unlock = (UnlockMulti)(nil)

func (m UnlockMulti) Diagnostics(diags tfdiags.Diagnostics) {
	for _, o := range m {
		o.Diagnostics(diags)
	}
}

func (m UnlockMulti) LockingDisabledForBackend() {
	for _, o := range m {
		o.LockingDisabledForBackend()
	}
}

func (m UnlockMulti) CannotUnlockByAnotherProcess() {
	for _, o := range m {
		o.CannotUnlockByAnotherProcess()
	}
}

func (m UnlockMulti) ForceUnlockCancelled() {
	for _, o := range m {
		o.ForceUnlockCancelled()
	}
}

func (m UnlockMulti) ForceUnlockSucceeded() {
	for _, o := range m {
		o.ForceUnlockSucceeded()
	}
}

type UnlockHuman struct {
	view *View
}

var _ Unlock = (*UnlockHuman)(nil)

func (v *UnlockHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *UnlockHuman) LockingDisabledForBackend() {
	v.view.errorln("Locking is disabled for this backend")
}

func (v *UnlockHuman) CannotUnlockByAnotherProcess() {
	v.view.errorln("Local state cannot be unlocked by another process")
}

func (v *UnlockHuman) ForceUnlockCancelled() {
	_, _ = v.view.streams.Println("force-unlock cancelled.")
}

func (v *UnlockHuman) ForceUnlockSucceeded() {
	const outputUnlockSuccess = `[reset][bold][green]OpenTofu state has been successfully unlocked![reset][green]

The state has been unlocked, and OpenTofu commands should now be able to
obtain a new lock on the remote state.`
	_, _ = v.view.streams.Println(v.view.colorize.Color(outputUnlockSuccess))
}

type UnlockJSON struct {
	view *JSONView
}

var _ Unlock = (*UnlockJSON)(nil)

func (v *UnlockJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *UnlockJSON) LockingDisabledForBackend() {
	v.view.Error("Locking is disabled for this backend")
}

func (v *UnlockJSON) CannotUnlockByAnotherProcess() {
	v.view.Error("Local state cannot be unlocked by another process")
}

func (v *UnlockJSON) ForceUnlockCancelled() {
	v.view.Info("force-unlock cancelled")
}

func (v *UnlockJSON) ForceUnlockSucceeded() {
	v.view.Info("The state has been unlocked, and OpenTofu commands should now be able to obtain a new lock on the remote state.")
}
