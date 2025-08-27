// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"

	"github.com/apparentlymart/go-versions/versions"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ConfigInstance represents an instance ofan already-assembled configuration
// tree, bound to some input variable values and other context that were
// provided when it was built.
type ConfigInstance struct {
	rootModuleInstance *configgraph.ModuleInstance
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
	// In typical use the InputValues map is assembled based on a combination
	// of ".tfvars" files, CLI arguments, and environment variables, but that's
	// the responsibility of the Tofu CLI layer and so this package is totally
	// unopinionated about how those are provided, so e.g. for .tftest.hcl "run"
	// blocks the input values could come from the test scenario configuration
	// instead.
	//
	// In unit tests where the source of input variables is immaterial,
	// [InputValuesForTesting] might be useful to build values for this
	// field inline in the test code.
	InputValues map[addrs.InputVariable]exprs.Valuer

	// EvalContext describes the context where the call is being made, dealing
	// with cross-cutting concerns like which providers are available and how
	// to load them.
	EvalContext *EvalContext
}

// NewConfigInstance builds a new [ConfigInstance] based on the information
// in the given [ConfigCall] object.
//
// If the returned diagnostics has errors then the first result is invalid
// and must not be used. Diagnostics returned directly by this function
// are focused only on the process of obtaining the root module; all other
// problems are deferred until subsequent evaluation.
func NewConfigInstance(ctx context.Context, call *ConfigCall) (*ConfigInstance, tfdiags.Diagnostics) {
	// The following compensations are for the convenience of unit tests, but
	// real callers should explicitly set all of this.
	if call.EvalContext == nil {
		call.EvalContext = &EvalContext{}
	}
	call.EvalContext.init()

	evalCtx := call.EvalContext

	rootModule, diags := evalCtx.Modules.ModuleConfig(ctx, call.RootModuleSource, versions.All, nil)
	if diags.HasErrors() {
		return nil, diags
	}
	rootModuleCall := &moduleInstanceCall{
		inputValues: call.InputValues,
	}
	rootModuleInstance := compileModuleInstance(rootModule, call.RootModuleSource, rootModuleCall, evalCtx)
	return &ConfigInstance{
		rootModuleInstance: rootModuleInstance,
	}, nil
}
