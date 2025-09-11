// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package evalglue

import (
	"context"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// UncompiledModule is an interface implemented by objects representing a
// module that has not yet been "compiled" into a [CompiledModuleInstance].
//
// This provides the facilities needed to validate a module call in preparation
// for using its settings to compile the module, and then for finally
// actually compiling the module into a [CompiledModuleInstance].
//
// This interface abstracts over potentially multiple different implementations
// representing different editions of the OpenTofu language, and so describes
// the API that all editions should agree on to allow modules of different
// editions to coexist and collaborate in a single configuration tree.
type UncompiledModule interface {
	// ValidateModuleInputs checks whether the given value is suitable to be
	// used as the "inputs" when instantiating the module.
	//
	// In the current language "inputsVal" should always be of an object type
	// whose attributes correspond to the input variables declared inside
	// the module.
	ValidateModuleInputs(ctx context.Context, inputsVal cty.Value) tfdiags.Diagnostics

	// TODO: Something similar to ValidateInputs for the "providers sidechannel"
	// so we can know what providers the child module is expecting to be passed
	// and thus know what's supposed to be in the "providers" argument of the
	// calling module block? Annoying to expose that directly as part of this
	// abstraction but probably the most pragmatic way to do it as long as
	// the providers sidechannel continues to exist.

	// ModuleOutputsTypeConstraint returns the type constraint that the
	// outputs value produced by all instances of this module would conform
	// to. It's always valid to use the result with `cty.UnknownVal` to produce
	// a placeholder for the result of an instance of this module.
	//
	// In the current language this should always describe an object type whose
	// attributes correspond to the output values declared in the module.
	ModuleOutputsTypeConstraint(ctx context.Context) cty.Type

	// CompileModuleInstance uses the given [ModuleCall] to attempt to compile
	// the module into a [CompiledModuleInstance], or returns error diagnostics
	// explaining why that isn't possible.
	//
	// calleeAddr is the module instance address that the newly-compiled module
	// appears at. This affects how the module describes itself and the objects
	// within it in the global address space.
	CompileModuleInstance(ctx context.Context, calleeAddr addrs.ModuleInstance, call *ModuleCall) (CompiledModuleInstance, tfdiags.Diagnostics)
}

// ModuleCall represents the information needed to instantiate [UncompiledModule]
// into [CompiledModuleInstance].
type ModuleCall struct {
	// InputValues describes the inputs to the module.
	//
	// The value produced by this valuer should have been previously
	// successfully validated using [UncompiledModule.ValidateModuleInputs],
	// or compilation is likely to fail with a potentially-confusing error.
	InputValues exprs.Valuer

	// AllowImpureFunctions controls whether to allow full use of a small
	// number of functions that produce different results each time they are
	// called, such as "timestamp".
	//
	// This should typically just be passed on verbatim from an equivalent
	// setting in the parent module, because all module instances in a
	// configuration instance should agree about whether impure functions
	// are active.
	AllowImpureFunctions bool

	EvaluationGlue Glue

	// EvalContext describes the context where the call is being made, dealing
	// with cross-cutting concerns like which providers are available and how
	// to load them.
	//
	// This should typically just be passed on verbatim from an equivalent
	// setting in the parent module, because EvalContext holds cross-cutting
	// concerns from the environment in which OpenTofu is running.
	EvalContext *EvalContext
}
