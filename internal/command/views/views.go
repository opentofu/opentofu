// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

// ErrUnsupportedLocalOp is the common error message shown for operations
// that require a backend.Local.
const errUnsupportedLocalOp = `The configured backend doesn't support this operation.

The "backend" in OpenTofu defines how OpenTofu operates. The default
backend performs all operations locally on your machine. Your configuration
is configured to use a non-local backend. This backend doesn't support this
operation.
`
