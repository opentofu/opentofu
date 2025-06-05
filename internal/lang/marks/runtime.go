// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package marks

// Runtime is a mark used only during init-time evaluation to annotate any
// unknown value that's acting as a placeholder for a value that won't be
// available until runtime.
//
// This is a singleton mark for prototyping purposes, but a real implementation
// should track the address of the runtime object that the value is representing
// so that error messages can specify exactly what isn't available during the
// init phase, rather than just making a vague statement.
const Runtime = valueMark("runtime")
