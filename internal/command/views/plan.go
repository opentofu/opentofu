// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

// The Plan view is used for the plan command.
type Plan interface {
	Operation() Operation
	Hooks() []tofu.Hook

	Diagnostics(diags tfdiags.Diagnostics)
	HelpPrompt()
}

// NewPlan returns an initialized Plan implementation for the given ViewType.
func NewPlan(vt arguments.ViewType, view *View) Plan {
	switch vt {
	case arguments.ViewJSON:
		return &PlanJSON{
			view: NewJSONView(view),
		}
	case arguments.ViewHuman:
		return &PlanHuman{
			view:         view,
			inAutomation: view.RunningInAutomation(),
		}
	default:
		panic(fmt.Sprintf("unknown view type %v", vt))
	}
}

// The PlanHuman implementation renders human-readable text logs, suitable for
// a scrolling terminal.
type PlanHuman struct {
	view *View

	inAutomation bool
}

var _ Plan = (*PlanHuman)(nil)

func (v *PlanHuman) Operation() Operation {
	return NewOperation(arguments.ViewHuman, v.inAutomation, v.view)
}

func (v *PlanHuman) Hooks() []tofu.Hook {
	return []tofu.Hook{NewUIOptionalHook(v.view)}
}

func (v *PlanHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *PlanHuman) HelpPrompt() {
	v.view.HelpPrompt("plan")
}

// The PlanJSON implementation renders streaming JSON logs, suitable for
// integrating with other software.
type PlanJSON struct {
	view *JSONView
}

var _ Plan = (*PlanJSON)(nil)

func (v *PlanJSON) Operation() Operation {
	return &OperationJSON{view: v.view}
}

func (v *PlanJSON) Hooks() []tofu.Hook {
	return []tofu.Hook{
		newJSONHook(v.view),
	}
}

func (v *PlanJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *PlanJSON) HelpPrompt() {
}
