// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import "github.com/opentofu/opentofu/internal/tfdiags"

// Root is the view that is meant to be used strictly during the initialisation of the process
// and offers methods to print errors as raw as possible.
type Root struct {
	v *View
}

func NewRoot(v *View) *Root {
	return &Root{v: v}
}

func (v *Root) Error(msg string) {
	_, _ = v.v.streams.Eprintln(msg)
}

func (v *Root) Diagnostics(diagnostics tfdiags.Diagnostics) {
	v.v.Diagnostics(diagnostics)
}
