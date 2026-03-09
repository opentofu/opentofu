// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type State interface {
	Diagnostics(diags tfdiags.Diagnostics)

	// General `state` output
	StateNotFound()
	StateLoadingFailure(baseError string)
	StateSavingError(baseError string)

	// `tofu state list` specific
	StateListAddr(resAddr addrs.AbsResourceInstance)

	// `tofu state mv` specific
	ErrorMovingToAlreadyExistingDst()
	ResourceMoveStatus(dryRun bool, src, dest string)
	DryRunMovedStatus(moved int)
	MoveFinalStatus(moved int)

	// `tofu state pull` specific
	PrintPulledState(state string)
}

// NewState returns an initialized State implementation for the given ViewType.
func NewState(args arguments.ViewOptions, view *View) State {
	var ret State
	switch args.ViewType {
	case arguments.ViewJSON:
		ret = &StateJSON{view: NewJSONView(view, nil)}
	case arguments.ViewHuman:
		ret = &StateHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewType))
	}

	if args.JSONInto != nil {
		ret = &StateMulti{ret, &StateJSON{view: NewJSONView(view, args.JSONInto)}}
	}
	return ret
}

type StateMulti []State

var _ State = (StateMulti)(nil)

func (m StateMulti) Diagnostics(diags tfdiags.Diagnostics) {
	for _, o := range m {
		o.Diagnostics(diags)
	}
}

func (m StateMulti) StateNotFound() {
	for _, o := range m {
		o.StateNotFound()
	}
}

func (m StateMulti) StateLoadingFailure(baseError string) {
	for _, o := range m {
		o.StateLoadingFailure(baseError)
	}
}

func (m StateMulti) StateSavingError(baseError string) {
	for _, o := range m {
		o.StateSavingError(baseError)
	}
}

func (m StateMulti) StateListAddr(resAddr addrs.AbsResourceInstance) {
	for _, o := range m {
		o.StateListAddr(resAddr)
	}
}

func (m StateMulti) ErrorMovingToAlreadyExistingDst() {
	for _, o := range m {
		o.ErrorMovingToAlreadyExistingDst()
	}
}

func (m StateMulti) ResourceMoveStatus(dryRun bool, src, dest string) {
	for _, o := range m {
		o.ResourceMoveStatus(dryRun, src, dest)
	}
}

func (m StateMulti) DryRunMovedStatus(moved int) {
	for _, o := range m {
		o.DryRunMovedStatus(moved)
	}
}

func (m StateMulti) MoveFinalStatus(moved int) {
	for _, o := range m {
		o.MoveFinalStatus(moved)
	}
}

func (m StateMulti) PrintPulledState(state string) {
	for _, o := range m {
		o.PrintPulledState(state)
	}
}

type StateHuman struct {
	view *View
}

var _ State = (*StateHuman)(nil)

func (v *StateHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *StateHuman) StateNotFound() {
	v.view.errorln(errStateNotFound)
}

func (v *StateHuman) StateLoadingFailure(baseError string) {
	v.view.errorln(fmt.Sprintf(errStateLoadingState, baseError))
}

func (v *StateHuman) StateSavingError(baseError string) {
	v.view.errorln(fmt.Sprintf(errStateRmPersist, baseError))
}

func (v *StateHuman) StateListAddr(resAddr addrs.AbsResourceInstance) {
	_, _ = v.view.streams.Println(resAddr.String())
}

func (v *StateHuman) ErrorMovingToAlreadyExistingDst() {
	v.view.errorln(errStateMvDstExists)
}

func (v *StateHuman) ResourceMoveStatus(dryRun bool, src, dest string) {
	if dryRun {
		_, _ = v.view.streams.Println(fmt.Sprintf("Would move %q to %q", src, dest))
		return
	}
	_, _ = v.view.streams.Println(fmt.Sprintf("Move %q to %q", src, dest))
}

func (v *StateHuman) DryRunMovedStatus(moved int) {
	if moved == 0 {
		_, _ = v.view.streams.Println("Would have moved nothing.")
	}
}

func (v *StateHuman) MoveFinalStatus(moved int) {
	if moved == 0 {
		_, _ = v.view.streams.Println("No matching objects found.")
		return
	}
	_, _ = v.view.streams.Println(fmt.Sprintf("Successfully moved %d object(s).", moved))
}

func (v *StateHuman) PrintPulledState(state string) {
	_, _ = v.view.streams.Println(state)
}

type StateJSON struct {
	view *JSONView
}

var _ State = (*StateJSON)(nil)

func (v *StateJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *StateJSON) StateNotFound() {
	cleanedUp := strings.ReplaceAll(
		strings.ReplaceAll(errStateNotFound, "\n", " "),
		"  ", " ",
	)
	v.view.Error(cleanedUp)
}

func (v *StateJSON) StateLoadingFailure(baseError string) {
	cleanedUp := strings.ReplaceAll(
		strings.ReplaceAll(errStateLoadingState, "\n", " "),
		"  ", " ",
	)
	if baseError != "" && !strings.HasSuffix(baseError, ".") {
		baseError += "."
	}
	msg := fmt.Sprintf(cleanedUp, baseError)
	v.view.Error(msg)
}

func (v *StateJSON) StateSavingError(baseError string) {
	cleanedUp := strings.ReplaceAll(
		strings.ReplaceAll(errStateRmPersist, "\n", " "),
		"  ", " ",
	)
	if baseError != "" && !strings.HasSuffix(baseError, ".") {
		baseError += "."
	}
	msg := fmt.Sprintf(cleanedUp, baseError)
	v.view.Error(msg)
}

func (v *StateJSON) StateListAddr(resAddr addrs.AbsResourceInstance) {
	v.view.log.Info(resAddr.String(), "type", "resource_address")
}

func (v *StateJSON) ErrorMovingToAlreadyExistingDst() {
	cleanedUp := strings.ReplaceAll(
		strings.ReplaceAll(errStateMvDstExists, "\n", " "),
		"  ", " ",
	)
	v.view.Error(cleanedUp)
}

func (v *StateJSON) ResourceMoveStatus(dryRun bool, src, dest string) {
	if dryRun {
		v.view.Info(fmt.Sprintf("Would move %q to %q", src, dest))
		return
	}
	v.view.Info(fmt.Sprintf("Move %q to %q", src, dest))
}

func (v *StateJSON) DryRunMovedStatus(moved int) {
	if moved == 0 {
		v.view.Info("Would have moved nothing")
	}
}

func (v *StateJSON) MoveFinalStatus(moved int) {
	if moved == 0 {
		v.view.Info("No matching objects found")
		return
	}
	v.view.Info(fmt.Sprintf("Successfully moved %d object(s)", moved))
}

func (v *StateJSON) PrintPulledState(_ string) {
	v.view.Error("printing the pulled state is not available in the JSON view. The `tofu state pull` should not be configured with the `-json` flag")
}

const errStateLoadingState = `Error loading the state: %[1]s

Please ensure that your OpenTofu state exists and that you've
configured it properly. You can use the "-state" flag to point
OpenTofu at another state file.`

const errStateNotFound = `No state file was found!

State management commands require a state file. Run this command
in a directory where OpenTofu has been run or use the -state flag
to point the command to a specific state location.`

const errStateMvDstExists = `Error moving state: destination module already exists.

Please ensure your addresses and state paths are valid. No
state was persisted. Your existing states are untouched.`

const errStateRmPersist = `Error saving the state: %s

The state was not saved. No items were removed from the persisted
state. No backup was created since no modification occurred. Please
resolve the issue above and try again.`
