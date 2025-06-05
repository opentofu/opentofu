// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/format"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

// The DriftCheck view is used for the drift-check command.
type DriftCheck interface {
	ResourceCount(stateOutPath string)
	Outputs(outputValues map[string]*states.OutputValue)

	Operation() Operation
	Hooks() []tofu.Hook

	Diagnostics(diags tfdiags.Diagnostics)
	HelpPrompt()
}

// NewDriftCheck returns an initialized DriftCheck implementation for the given ViewType.
func NewDriftCheck(view *View) DriftCheck {
	return &DriftCheckHuman{
		view:         view,
		inAutomation: view.RunningInAutomation(),
		countHook:    &countHook{},
	}
}

// The DriftCheckHuman implementation renders human-readable text logs, suitable for
// a scrolling terminal.
type DriftCheckHuman struct {
	view *View

	destroy      bool
	inAutomation bool

	countHook *countHook
}

var _ DriftCheck = (*DriftCheckHuman)(nil)

func (v *DriftCheckHuman) ResourceCount(stateOutPath string) {
	// TODO: Use this refactoring on the apply function
	msg := "[reset][bold][green]\nDrift Check complete! Resources: "

	var actions []string
	if v.countHook.Imported > 0 {
		actions = append(actions, fmt.Sprintf("%d imported", v.countHook.Imported))
	}

	actions = append(actions, fmt.Sprintf("%d added", v.countHook.Added))
	actions = append(actions, fmt.Sprintf("%d changed", v.countHook.Changed))
	actions = append(actions, fmt.Sprintf("%d destroyed", v.countHook.Removed))

	if v.countHook.Forgotten > 0 {
		actions = append(actions, fmt.Sprintf("%d forgotten", v.countHook.Forgotten))
	}
	msg = fmt.Sprintf("%s %s.", msg, strings.Join(actions, ", "))
	v.view.streams.Printf(v.view.colorize.Color(msg))

	if (v.countHook.Added > 0 || v.countHook.Changed > 0) && stateOutPath != "" {
		v.view.streams.Printf("\n%s\n\n", format.WordWrap(stateOutPathPostApply, v.view.outputColumns()))
		v.view.streams.Printf("State path: %s\n", stateOutPath)
	}
}

func (v *DriftCheckHuman) Outputs(outputValues map[string]*states.OutputValue) {
	if len(outputValues) > 0 {
		v.view.streams.Print(v.view.colorize.Color("[reset][bold][green]\nOutputs:\n\n"))
		NewOutput(arguments.ViewHuman, v.view).Output("", outputValues)
	}
}

func (v *DriftCheckHuman) Operation() Operation {
	return NewOperation(arguments.ViewHuman, v.inAutomation, v.view)
}

func (v *DriftCheckHuman) Hooks() []tofu.Hook {
	return []tofu.Hook{v.countHook, NewUIOptionalHook(v.view)}
}

func (v *DriftCheckHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *DriftCheckHuman) HelpPrompt() {
	command := "drift-check"
	v.view.HelpPrompt(command)
}
