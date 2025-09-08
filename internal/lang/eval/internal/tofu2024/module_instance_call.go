// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type ModuleInstanceCall struct {
	// CalleeAddr is the absolute address of this module instance that should
	// be used as the basis for calculating addresses of resources within
	// and beneath it.
	CalleeAddr addrs.ModuleInstance

	// DeclRange is the source location of the header of the module block
	// that is making this call, or some similar config construct that's
	// acting like a module call.
	//
	// This should be nil for calls that are caused by something other than
	// configuration, such as a top-level call to a root module caused by
	// running an OpenTofu CLI command.
	DeclRange *tfdiags.SourceRange

	// InputValues describes how to build the values for the input variables
	// for this instance of the module.
	//
	// This is a single valuer because when constructing this the caller
	// doesn't yet know what input variables the child module is expecting,
	// and so it just sends over whatever was present in the call and expects
	// the callee to reject it if it isn't compatible with the callee's API.
	InputValues exprs.Valuer

	// ProvidersFromParent are values representing provider instances passed in
	// through our side-channel using the "providers" meta argument in the
	// calling module block.
	//
	// These valuers MUST return values of types returned by
	// [configgraph.ProviderInstanceRefType], which are capsule types that
	// carry [configgraph.ProviderInstance] values. It's implemented this
	// way so that [configgraph] can think of provider instance references as
	// just normal values and not be aware of the current weird situation where
	// they have their own special reference expression syntax and pass
	// between modules via completely different rules than other values.
	//
	// (One day we'd like to actually offer provider instance references as
	// normal values in the surface language too, but it's not obvious how
	// to get there from our current language without splitting the ecosystem
	// between old-style and new-style modules.)
	ProvidersFromParent map[addrs.LocalProviderConfig]exprs.Valuer

	// AllowImpureFunctions controls whether to allow full use of a small
	// number of functions that produce different results each time they are
	// called, such as "timestamp". This should be set to true only during
	// the apply phase and in some more contrived situations such as in the
	// "tofu console" command's REPL.
	//
	// This setting should typically be propagated verbatim to any child
	// module instances because there's little reason for one module to
	// allow impure functions while another does not.
	AllowImpureFunctions bool

	// EvaluationGlue is the [evalconfig.Glue] implementation to use when
	// the evaluator needs information from outside of the configuration.
	//
	// All module instances belonging to a single configuration tree should
	// typically share the same evaluationGlue.
	EvaluationGlue evalglue.Glue

	// EvalContext is the [EvalContext] to use to interact with the context.
	//
	// Compared to evaluationGlue, evalContext deals with concerns that
	// are typically held constant throughout sequential validate, plan, and
	// apply phases, whereas evaluationGlue is where we deal with behaviors
	// that need to vary between phases.
	EvalContext *evalglue.EvalContext
}
