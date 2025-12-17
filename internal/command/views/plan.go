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
func NewPlan(args *arguments.Plan, view *View) Plan {
	switch args.ViewOptions.ViewType {
	case arguments.ViewJSON:
		return &PlanJSON{
			view: NewJSONView(view, nil),
		}
	case arguments.ViewHuman:
		human := &PlanHuman{
			view:         view,
			inAutomation: view.RunningInAutomation(),
		}

		if args.ViewOptions.JSONInto != nil {
			return PlanMulti{human, &PlanJSON{view: NewJSONView(view, args.ViewOptions.JSONInto)}}
		}

		return human
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewOptions.ViewType))
	}
}

type PlanMulti []Plan

var _ Plan = (PlanMulti)(nil)

func (p PlanMulti) Operation() Operation {
	var operation OperationMulti
	for _, plan := range p {
		operation = append(operation, plan.Operation())
	}
	return operation
}

func (p PlanMulti) Hooks() []tofu.Hook {
	var hooks []tofu.Hook
	for _, plan := range p {
		hooks = append(hooks, plan.Hooks()...)
	}
	return hooks
}

func (p PlanMulti) Diagnostics(diags tfdiags.Diagnostics) {
	for _, plan := range p {
		plan.Diagnostics(diags)
	}
}

func (p PlanMulti) HelpPrompt() {
	for _, plan := range p {
		plan.HelpPrompt()
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
