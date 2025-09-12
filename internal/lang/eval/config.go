// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"

	"github.com/apparentlymart/go-versions/versions"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ConfigInstance represents the combination of a configuration and some
// input variables used to call into its root module, along with related
// context such as available providers and module packages.
type ConfigInstance struct {
	rootModuleSource     addrs.ModuleSource
	inputValues          exprs.Valuer
	evalContext          *evalglue.EvalContext
	allowImpureFunctions bool
}

// ConfigCall describes a call to a root module that acts conceptually like
// a "module" block but is instead implied by something outside of the
// module language itself, such as running an OpenTofu CLI command.
type ConfigCall struct {
	// RootModuleSource is the source address of the root module.
	//
	// This must be a source address that can be resolved by the
	// [ExternalModules] implementation provided in EvalContext.
	RootModuleSource addrs.ModuleSource

	// InputValues describes how to obtain values for the input variables
	// declared in the root module.
	//
	// In typical use the InputValues object is assembled based on a combination
	// of ".tfvars" files, CLI arguments, and environment variables, but that's
	// the responsibility of the Tofu CLI layer and so this package is totally
	// unopinionated about how those are provided, so e.g. for .tftest.hcl "run"
	// blocks the input values could come from the test scenario configuration
	// instead.
	//
	// In unit tests where the source of input variables is immaterial,
	// [InputValuesForTesting] might be useful to build values for this
	// field inline in the test code.
	InputValues exprs.Valuer

	// AllowImpureFunctions controls whether to allow full use of a small
	// number of functions that produce different results each time they are
	// called, such as "timestamp". This should be set to true only during
	// the apply phase and in some more contrived situations such as in the
	// "tofu console" command's REPL.
	AllowImpureFunctions bool

	// EvalContext describes the context where the call is being made, dealing
	// with cross-cutting concerns like which providers are available and how
	// to load them.
	EvalContext *evalglue.EvalContext
}

// NewConfigInstance builds a new [ConfigInstance] based on the information
// in the given [ConfigCall] object.
//
// If the returned diagnostics has errors then the first result is invalid
// and must not be used.
//
// Note that this function focuses only on checking that the call itself seems
// sensible, and does not perform any immediate evaluation of the configuration,
// so success of this function DOES NOT imply that the configuration is valid.
// Use methods of a valid [`ConfigInstance`] produced by this function to
// gather more information about the configuration.
func NewConfigInstance(ctx context.Context, call *ConfigCall) (*ConfigInstance, tfdiags.Diagnostics) {
	call.EvalContext.AssertValid()

	inst := &ConfigInstance{
		rootModuleSource:     call.RootModuleSource,
		inputValues:          call.InputValues,
		allowImpureFunctions: call.AllowImpureFunctions,
		evalContext:          call.EvalContext,
	}

	// We currently don't do any other early work here and instead just wait
	// until we're asked a more specific question using one of the methods
	// of the result. If that continues to be true then perhaps we'll drop
	// the tfdiags.Diagnostics result from this function to be clearer
	// that it's purely a constructor and doesn't do any "real work".
	return inst, nil
}

// EvalContext returns the [EvalContext] that the [ConfigInstance] would use
// to interact with its surrounding environment.
//
// This is exposed so that other systems that do work alongside the
// [ConfigInstance] work, such as implementations of [PlanGlue], can guarantee
// that they are interacting with the environment in a consistent way.
func (c *ConfigInstance) EvalContext() *EvalContext {
	return c.evalContext
}

// newRootModuleInstance prepares a [configgraph.ModuleInstance] object based
// on the [ConfigInstance] and the given evaluation glue, as a shared building
// block for various exported methods on [ConfigInstance].
//
// This returns diagnostics encountered when loading the root module, but
// does not perform any further checks. Callers must then drive evaluation of
// the resulting configuration tree by calling
// [configgraph.ModuleInstance.CheckBeneath] to ensure that everything in
// the configuration gets a chance to report errors.
func (c *ConfigInstance) newRootModuleInstance(ctx context.Context, glue evalglue.Glue) (evalglue.CompiledModuleInstance, tfdiags.Diagnostics) {
	rootModule, diags := c.evalContext.Modules.ModuleConfig(ctx, c.rootModuleSource, versions.All, nil)
	if diags.HasErrors() {
		return nil, diags
	}
	ret, moreDiags := rootModule.CompileModuleInstance(ctx, addrs.RootModuleInstance, &evalglue.ModuleCall{
		InputValues:          c.inputValues,
		AllowImpureFunctions: c.allowImpureFunctions,
		EvaluationGlue:       glue,
		EvalContext:          c.evalContext,
	})
	diags = diags.Append(moreDiags)
	return ret, diags
}
