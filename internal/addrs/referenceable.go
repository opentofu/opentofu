// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"github.com/zclconf/go-cty/cty"
)

// Referenceable is an interface implemented by all address types that can
// appear as references in configuration language expressions.
type Referenceable interface {
	// All implementations of this interface must be covered by the type switch
	// in lang.Scope.buildEvalContext.
	referenceableSigil()

	// All Referenceable address types must have unique keys.
	UniqueKeyer

	// Path returns a [cty.Path] representation of the reference.
	//
	// The implementation must either allocate a new backing array for each
	// call or ensure that the returned array has no excess capacity, so that
	// callers can safely append additional steps to it without causing
	// a data race on the same backing array capacity.
	Path() cty.Path

	// String produces a string representation of the address that could be
	// parsed as a HCL traversal and passed to ParseRef to produce an identical
	// result.
	String() string
}

type referenceable struct {
}

func (r referenceable) referenceableSigil() {
}
