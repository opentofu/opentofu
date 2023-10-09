// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

// UIOutput is the interface that must be implemented to output
// data to the end user.
type UIOutput interface {
	Output(string)
}
