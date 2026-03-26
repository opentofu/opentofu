// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ProvidersSchema is the interface for the providers schema view.
type ProvidersSchema interface {
	// Diagnostics is used to render diagnostic messages out to the user.
	Diagnostics(diags tfdiags.Diagnostics)

	// UnsupportedLocalOp is used to output a message to the user that the current operation is unsupported locally.
	UnsupportedLocalOp()

	// Output is used to display the final JSON output of the providers schema map.
	Output(json string)
}

// NewProvidersSchema creates a new ProvidersSchema view.
func NewProvidersSchema(v *View) ProvidersSchema {
	return &ProvidersSchemaMixed{view: v}
}

type ProvidersSchemaMixed struct {
	view *View
}

func (v *ProvidersSchemaMixed) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *ProvidersSchemaMixed) UnsupportedLocalOp() {
	v.view.Diagnostics(tfdiags.Diagnostics{diagUnsupportedLocalOp})
}

func (v *ProvidersSchemaMixed) Output(json string) {
	// The provider schema output is expected to just be the raw JSON
	_, _ = v.view.streams.Println(json)
}
