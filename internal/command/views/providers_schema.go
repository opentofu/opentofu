// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ProvidersSchema is the interface for the providers schema view.
type ProvidersSchema interface {
	// Diagnostics is used to render diagnostic messages out to the user.
	Diagnostics(diags tfdiags.Diagnostics)

	// HelpPrompt is used to output a message to the user that they can use the -help flag.
	HelpPrompt()

	// UnsupportedLocalOp is used to output a message to the user that the current operation is unsupported locally.
	UnsupportedLocalOp()

	// Output is used to display the final JSON output of the providers schema map.
	Output(json string)
}

// NewProvidersSchema creates a new ProvidersSchema view.
func NewProvidersSchema(opts arguments.ViewOptions, v *View) ProvidersSchema {
	return &ProvidersSchemaMixed{view: v}
}

type ProvidersSchemaMixed struct {
	view *View
}

func (v *ProvidersSchemaMixed) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *ProvidersSchemaMixed) HelpPrompt() {
	v.view.HelpPrompt("providers schema")
}

func (v *ProvidersSchemaMixed) UnsupportedLocalOp() {
	v.view.errorln(errUnsupportedLocalOp)
}

func (v *ProvidersSchemaMixed) Output(json string) {
	// The provider schema output is expected to just be the raw JSON
	v.view.streams.Println(json)
}
