// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package modules

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type WithSourceRange[T any] struct {
	Value       T
	SourceRange tfdiags.SourceRange
}

func withHCLSourceRange[T any](v T, rng hcl.Range) WithSourceRange[T] {
	return WithSourceRange[T]{
		Value:       v,
		SourceRange: tfdiags.SourceRangeFromHCL(rng),
	}
}

func withSourceRange[T any](v T, rng tfdiags.SourceRange) WithSourceRange[T] {
	return WithSourceRange[T]{
		Value:       v,
		SourceRange: rng,
	}
}
