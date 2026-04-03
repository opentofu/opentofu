// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Graph interface {
	Diagnostics(diags tfdiags.Diagnostics)
	ErrorUnsupportedLocalOp()
	Output(graphStr string)

	// Backend returns the non-command view that contains methods to provide
	// progress output for the backend operations.
	Backend() Backend
}

// NewGraph returns an initialized Graph implementation for the given ViewType.
func NewGraph(view *View) Graph {
	return &GraphHuman{view: view}
}

type GraphHuman struct {
	view *View
}

var _ Graph = (*GraphHuman)(nil)

func (v *GraphHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *GraphHuman) ErrorUnsupportedLocalOp() {
	v.Diagnostics(tfdiags.Diagnostics{diagUnsupportedLocalOp})
}

func (v *GraphHuman) Output(graphStr string) {
	_, _ = v.view.streams.Println(graphStr)
}

func (v *GraphHuman) Backend() Backend {
	return &BackendHuman{
		view: v.view,
	}
}
