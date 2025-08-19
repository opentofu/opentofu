// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

// graphNodeTemporaryValue is implemented by nodes that may represent temporary
// values, which are those not saved to the state file. This includes locals,
// variables, and non-root outputs.
// A boolean return value allows a node which may need to be saved to
// conditionally do so.
type graphNodeTemporaryValue interface {
	temporaryValue(op walkOperation) bool
}
