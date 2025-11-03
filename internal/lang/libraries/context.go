// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libraries

import (
	"context"
	"iter"

	"github.com/hashicorp/go-version"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// CompileContext is an interface implemented by callers of [Library.Compile] to
// integrate the loaded library with the rest of the system.
type CompileContext interface {
	// PredefinedFunctions returns all of the predefined ("built-in") functions
	// that are available to call from the definitions of values and functions
	// defined in a library.
	//
	// The function names should have no namespace prefix. This package will
	// automatically expose both unprefixed and "core::"-prefixed versions of
	// each described function, with the latter ensuring that the predefined
	// functions remain accessible even when shadowed by a function defined
	// inside the library.
	PredefinedFunctions(ctx context.Context) iter.Seq2[string, function.Function]

	// PredefinedTypes returns all of the predefined ("built-in") type
	// constructors that are available for use from the definitions in a
	// library.
	//
	// The type constructor names should have no namepace prefix. This package
	// will automatically expose both unprefixed and "core::"-prefixed versions
	// of each described type constructor, with the latter ensuring that the
	// predefined type constructors rmain accessible even when shadowed by
	// a type alias defined inside the library.
	PredefinedTypes(ctx context.Context) iter.Seq2[string, *TypeConstructor]

	// ProviderFunctions returns all of the functions that are defined by
	// the given provider.
	//
	// The compiler will only ask about providers that appeared in the
	// RequiredProviders field of the library description.
	//
	// The function names should have no namespace prefix. This package
	// will automatically generate the required prefix based on the local name
	// used to refer to the provider inside the library source code.
	//
	// The function implementations should call the function on an unconfigured
	// instance of the provider.
	ProviderFunctions(ctx context.Context, provider addrs.Provider) iter.Seq2[string, function.Function]

	// ChildLibrary compiles and returns another library that is a dependency
	// of the current library.
	//
	// The compiler will only ask for libraries that appeared in the
	// RequiredLibaries field of the library description. The same version
	// constraint that was used in RequiredLibraries is included again here
	// in case the library depends on the same library multiple times with
	// different version constraints, so the implementation can potentially
	// return a different version selection when given different version
	// constraints.
	ChildLibrary(ctx context.Context, sourceAddr addrs.ModuleSource, versions *version.Constraint) (*CompiledLibrary, tfdiags.Diagnostics)
}
