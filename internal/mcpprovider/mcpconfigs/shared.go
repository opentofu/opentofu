// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package mcpconfigs

import (
	"github.com/hashicorp/hcl/v2"
)

type WithRange[T any] struct {
	Value T
	Range hcl.Range
}

// withRange is a helper for constructing WithRange[T] values concisely using
// Go's automatic type inference.
func withRange[T any](value T, rng hcl.Range) WithRange[T] {
	return WithRange[T]{value, rng}
}
