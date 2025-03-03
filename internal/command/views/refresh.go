// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views/json"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

// The Refresh view is used for the refresh command.
type Refresh interface {
	Outputs(outputValues map[string]*states.OutputValue)

	Operation() Operation
	Hooks() []tofu.Hook

	Diagnostics(diags tfdiags.Diagnostics)
	HelpPrompt()
}

// NewRefresh returns an initialized Refresh implementation for the given ViewType.
func NewRefresh(vt arguments.ViewType, view *View) Refresh {
	switch vt {
	case arguments.ViewJSON:
		return &RefreshJSON{
			view: NewJSONView(view),
		}
	case arguments.ViewHuman:
		return &RefreshHuman{
			view:         view,
			inAutomation: view.RunningInAutomation(),
			countHook:    &countHook{},
		}
	default:
		panic(fmt.Sprintf("unknown view type %v", vt))
	}
}

// The RefreshHuman implementation renders human-readable text logs, suitable for
// a scrolling terminal.
type RefreshHuman struct {
	view *View

	inAutomation bool

	countHook *countHook
}

var _ Refresh = (*RefreshHuman)(nil)

func (v *RefreshHuman) Outputs(outputValues map[string]*states.OutputValue) {
	if len(outputValues) > 0 {
		v.view.streams.Print(v.view.colorize.Color("[reset][bold][green]\nOutputs:\n\n"))
		NewOutput(arguments.ViewHuman, v.view).Output("", outputValues)
	}
}

func (v *RefreshHuman) Operation() Operation {
	return NewOperation(arguments.ViewHuman, v.inAutomation, v.view)
}

func (v *RefreshHuman) Hooks() []tofu.Hook {
	return []tofu.Hook{v.countHook, NewUIOptionalHook(v.view)}
}

func (v *RefreshHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *RefreshHuman) HelpPrompt() {
	v.view.HelpPrompt("refresh")
}

// The RefreshJSON implementation renders streaming JSON logs, suitable for
// integrating with other software.
type RefreshJSON struct {
	view *JSONView
}

var _ Refresh = (*RefreshJSON)(nil)

func (v *RefreshJSON) Outputs(outputValues map[string]*states.OutputValue) {
	outputs, diags := json.OutputsFromMap(outputValues)
	if diags.HasErrors() {
		v.Diagnostics(diags)
	} else {
		v.view.Outputs(outputs)
	}
}

func (v *RefreshJSON) Operation() Operation {
	return &OperationJSON{view: v.view}
}

func (v *RefreshJSON) Hooks() []tofu.Hook {
	return []tofu.Hook{
		newJSONHook(v.view),
	}
}

func (v *RefreshJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *RefreshJSON) HelpPrompt() {
}
