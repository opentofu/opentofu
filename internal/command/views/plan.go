// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/repl"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

// The Plan view is used for the plan command.
type Plan interface {
	Operation() Operation
	Hooks() []tofu.Hook

	WatchResult(ref *addrs.AbsReference, v cty.Value)
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

func (v *PlanHuman) WatchResult(ref *addrs.AbsReference, val cty.Value) {
	// TODO: Do we need to synchronize this with the work done by the
	// UI hook returned by PlanHuman.Hooks? For now we assume that
	// short writes to our streams are done atomically and so this is
	// fine as long as everyone is making sure to perform exactly one
	// write per whole message.
	streams := v.view.streams
	if ref.Module.IsRoot() {
		f := v.view.colorize.Color("\n[blue]===[reset] [bold]WATCH %s[reset]\n  %s\n[blue]==================[reset]\n\n")
		streams.Printf(f, ref.DisplayString(), repl.FormatValue(val, 2))
	} else {
		f := v.view.colorize.Color("\n[blue]===[reset] [bold]WATCH %s[reset] in %s\n  %s\n[blue]==================[reset]\n\n")
		streams.Printf(f, ref.LocalReference().DisplayString(), ref.Module.String(), repl.FormatValue(val, 2))
	}
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

func (v *PlanJSON) WatchResult(ref *addrs.AbsReference, val cty.Value) {
	// Watches are currently experimental, so we won't expose them in this
	// machine-readable integration point.
}

func (v *PlanJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *PlanJSON) HelpPrompt() {
}
