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

// NewProvidersSchema creates a new ProvidersSchema view based on the view type specified in the options.
func NewProvidersSchema(opts arguments.ViewOptions, v *View) ProvidersSchema {
	if opts.ViewType == arguments.ViewJSON {
		return &ProvidersSchemaJSON{v}
	}

	// This is a bit of a special case because providers schema actually requires the
	// -json flag. So the "human" view here only exists to print diagnostics around
	// flag parsing failures or the missing -json flag.
	return &ProvidersSchemaHuman{view: v}
}

type ProvidersSchemaHuman struct {
	view *View
}

func (v *ProvidersSchemaHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *ProvidersSchemaHuman) HelpPrompt() {
	v.view.HelpPrompt("providers schema")
}

func (v *ProvidersSchemaHuman) UnsupportedLocalOp() {
	v.view.errorln(errUnsupportedLocalOp)
}

func (v *ProvidersSchemaHuman) Output(json string) {
	// This should not happen because if we are in this View type we've failed validation
	panic("can't produce output in human view for providers schema")
}

type ProvidersSchemaJSON struct {
	view *View
}

func (v *ProvidersSchemaJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *ProvidersSchemaJSON) HelpPrompt() {
}

func (v *ProvidersSchemaJSON) UnsupportedLocalOp() {
	v.view.errorln(errUnsupportedLocalOp)
}

func (v *ProvidersSchemaJSON) Output(json string) {
	// The provider schema output is expected to just be the raw JSON
	v.view.streams.Print(json)
}
