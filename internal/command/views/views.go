// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import "github.com/opentofu/opentofu/internal/tfdiags"

// diagUnsupportedLocalOp is the common error message shown for operations
// that require a backend.Local.
var diagUnsupportedLocalOp = tfdiags.Sourceless(
	tfdiags.Error,
	"The configured backend doesn't support this operation",
	`The "backend" in OpenTofu defines how OpenTofu operates. The default backend performs all operations locally on your machine. Your configuration is configured to use a non-local backend. This backend doesn't support this operation.`)
