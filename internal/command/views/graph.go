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

// The Graph view is used for the graph command.
type Graph interface {
	Output(graphStr string)

	Diagnostics(diags tfdiags.Diagnostics)
	HelpPrompt()
}

// NewGraph returns an initialized Graph implementation for the given ViewType.
func NewGraph(vt arguments.ViewType, view *View) Graph {
	switch vt {
	case arguments.ViewHuman:
		return &GraphHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", vt))
	}
}

type GraphHuman struct {
	view *View
}

var _ Graph = (*GraphHuman)(nil)

func (v *GraphHuman) Output(graphStr string) {
	v.view.streams.Println(graphStr)
}

func (v *GraphHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *GraphHuman) HelpPrompt() {
	v.view.HelpPrompt("graph")
}
