// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"io"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Fmt interface {
	Diagnostics(diags tfdiags.Diagnostics)
	UserOutputWriter() io.Writer
}

// NewFmt returns an initialized Fmt implementation.
func NewFmt(view *View) Fmt {
	return &FmtHuman{view: view}
}

type FmtHuman struct {
	view *View
}

var _ Fmt = (*FmtHuman)(nil)

func (v *FmtHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

// UserOutputWriter returns a [io.Writer] that uses the [FmtHuman.view] as a proxy to write
// the user facing information during formatting.
func (v *FmtHuman) UserOutputWriter() io.Writer {
	return &writer{
		writeFn: func(msg string) {
			_, _ = v.view.streams.Println(msg)
		},
	}
}
