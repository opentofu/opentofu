// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package schema

// Equal is an interface that checks for deep equality between two objects.
type Equal interface {
	Equal(interface{}) bool
}
