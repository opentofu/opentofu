// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import "github.com/opentofu/opentofu/internal/tfdiags"

// Basic represents the basic view that can perform basic printing actions
// as showing diagnostics. This is implemented by most of the view implementations
// and can be used as argument to allow access to the defined basic functionality.
type Basic interface {
	Diagnostics(diags tfdiags.Diagnostics)
}

// diagUnsupportedLocalOp is the common error message shown for operations
// that require a backend.Local.
var diagUnsupportedLocalOp = tfdiags.Sourceless(
	tfdiags.Error,
	"The configured backend doesn't support this operation",
	`The "backend" in OpenTofu defines how OpenTofu operates. The default backend performs all operations locally on your machine. Your configuration is configured to use a non-local backend. This backend doesn't support this operation.`)
