// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import "github.com/opentofu/opentofu/internal/tfdiags"

// Root is the view that is meant to be used strictly during the initialisation of the process
// and offers methods to print errors as raw as possible.
type Root struct {
	view *View
}

func NewRoot(view *View) *Root {
	return &Root{view: view}
}

func (v *Root) Error(msg string) {
	_, _ = v.view.streams.Eprintln(msg)
}

func (v *Root) Diagnostics(diagnostics tfdiags.Diagnostics) {
	v.view.Diagnostics(diagnostics)
}
