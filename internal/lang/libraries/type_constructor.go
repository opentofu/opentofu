// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libraries

import (
	"github.com/zclconf/go-cty/cty"
)

// TypeConstructor describes a function for constructing a type in terms
// of zero or more other types.
//
// This is used both for type aliases declared inside a library and for
// external type constructors that are either predeclared (for core types)
// or imported from other libraries.
//
// This is essentially the type equivalent of [function.FunctionSpec] from
// cty's function system.
type TypeConstructor struct {
	// ArgumentNames are terse identifiers for the type arguments expected
	// by the constructor function. The length of this slice defines how
	// many arguments are required.
	//
	// A zero-length (typically nil) ArgumentNames represents a type that
	// is referred to using just its name as a plain identifier, rather
	// than using the function call syntax.
	ArgumentNames []string

	// Construct builds a type in terms of the given arguments, whose length
	// must match the length of ArgumentNames or this function will panic.
	//
	// It's the caller's responsibility to ensure that the correct number
	// of arguments were provided, and return its own error if not.
	Construct func(args []cty.Type) cty.Type
}

// TODO: Do we also want to support type constructors with non-type expressions
// as arguments? For example, providerinst(aws) might represent "reference to
// an instance of whatever provider has the local name aws" if we continue
// down the path of letting a provider instance reference be just a special
// type of value, and in that case "aws" is just one of the keys in the
// required_providers table instead of being a type expression.
//
// The above also doesn't have any way to describe the "object" and "tuple"
// type constructors currently in the type expression language.
