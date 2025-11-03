// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libraries

import (
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// CompiledLibrary is the result of [Library.Compile], which binds the library
// to an evaluation context so that its declarations can be used by some caller.
//
// The API of this type assumes that uses of the library were already validated
// using the [Library] object before compilation, and so this type exposes
// only the final compiled versions of the available symbols and not any of
// the associated metadata like declaration source locations.
type CompiledLibrary struct {
}

// Value returns the value associated with the given name inside the library,
// or [cty.NilVal] if there is no exported value of the given name.
func (l *CompiledLibrary) Value(name string) cty.Value {
	panic("unimplemented")
}

// Function returns the specification for the function of the given name inside
// the library, or nil if there is no exported function of the given name.
func (l *CompiledLibrary) Function(name string) *function.Spec {
	panic("unimplemented")
}

// TypeAlias returns a constructor for the type alias of the given name inside
// the library, or nil if there is no type alias of the given name.
func (l *CompiledLibrary) TypeAlias(name string) *TypeConstructor {
	panic("unimplemented")
}
