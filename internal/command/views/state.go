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
	StateLoadingFailure(baseError string)
	StateNotFound()

	StateListAddr(resAddr addrs.AbsResourceInstance)
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

func (m StateMulti) StateLoadingFailure(baseError string) {
	for _, o := range m {
		o.StateLoadingFailure(baseError)
	}
}

func (m StateMulti) StateNotFound() {
	for _, o := range m {
		o.StateNotFound()
	}
}

func (m StateMulti) StateListAddr(resAddr addrs.AbsResourceInstance) {
	for _, o := range m {
		o.StateListAddr(resAddr)
	}
}

type StateHuman struct {
	view *View
}

var _ State = (*StateHuman)(nil)

func (v *StateHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *StateHuman) StateLoadingFailure(baseError string) {
	v.view.errorln(fmt.Sprintf(errStateLoadingState, baseError))
}

func (v *StateHuman) StateNotFound() {
	v.view.errorln(errStateNotFound)
}

func (v *StateHuman) StateListAddr(resAddr addrs.AbsResourceInstance) {
	_, _ = v.view.streams.Println(resAddr.String())
}

type StateJSON struct {
	view *JSONView
}

var _ State = (*StateJSON)(nil)

func (v *StateJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
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

func (v *StateJSON) StateNotFound() {
	cleanedUp := strings.ReplaceAll(
		strings.ReplaceAll(errStateNotFound, "\n", " "),
		"  ", " ",
	)
	v.view.Error(cleanedUp)
}

func (v *StateJSON) StateListAddr(resAddr addrs.AbsResourceInstance) {
	v.view.log.Info(resAddr.String(), "type", "resource_address")
}

const errStateLoadingState = `Error loading the state: %[1]s

Please ensure that your OpenTofu state exists and that you've
configured it properly. You can use the "-state" flag to point
OpenTofu at another state file.`

const errStateNotFound = `No state file was found!

State management commands require a state file. Run this command
in a directory where OpenTofu has been run or use the -state flag
to point the command to a specific state location.`
