// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"iter"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/dag"
	"github.com/opentofu/opentofu/internal/linting/corelinting"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tracing"
	"github.com/zclconf/go-cty/cty"
)

// Validate performs semantic validation of a configuration, and returns
// any warnings or errors.
//
// Syntax and structural checks are performed by the configuration loader,
// and so are not repeated here.
//
// Validate considers only the configuration and so it won't catch any
// errors caused by current values in the state, or other external information
// such as root module input variables. However, the Plan function includes
// all of the same checks as Validate, in addition to the other work it does
// to consider the previous run state and the planning options.
func (c *Context) Validate(ctx context.Context, config *configs.Config) tfdiags.Diagnostics {
	defer c.acquireRun("validate")()

	var diags tfdiags.Diagnostics

	ctx, span := tracing.Tracer().Start(
		ctx, "Validation phase",
	)
	defer span.End()

	moreDiags := c.checkConfigDependencies(config)
	diags = diags.Append(moreDiags)
	// If required dependencies are not available then we'll bail early since
	// otherwise we're likely to just see a bunch of other errors related to
	// incompatibilities, which could be overwhelming for the user.
	if diags.HasErrors() {
		return diags
	}

	log.Printf("[DEBUG] Building and walking validate graph")

	// Validate is to check if the given module is valid regardless of
	// input values, current state, etc. Therefore we populate all of the
	// input values with unknown values of the expected type, allowing us
	// to perform a type check without assuming any particular values.
	varValues := make(InputValues)
	for name, variable := range config.Module.Variables {
		ty := variable.Type
		if ty == cty.NilType {
			// Can't predict the type at all, so we'll just mark it as
			// cty.DynamicVal (unknown value of cty.DynamicPseudoType).
			ty = cty.DynamicPseudoType
		}
		varValues[name] = &InputValue{
			Value:      cty.UnknownVal(ty),
			SourceType: ValueFromUnknown,
		}
	}

	// TEMP: Opt-in support for testing with the new experimental language
	// runtime. Refer to backend_temp_new_runtime.go for more information.
	if experimentalRuntimeEnabled() {
		moreDiags := c.newEngineValidate(ctx, config, varValues)
		return diags.Append(moreDiags)
	}

	importTargets := c.findImportTargets(config)

	providerFunctionTracker := make(ProviderFunctionMapping)

	graph, moreDiags := (&PlanGraphBuilder{
		Config:                  config,
		Plugins:                 c.plugins,
		State:                   states.NewState(),
		RootVariableValues:      varValues,
		Operation:               walkValidate,
		ProviderFunctionTracker: providerFunctionTracker,
		ImportTargets:           importTargets,
	}).Build(ctx, addrs.RootModuleInstance)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return diags
	}
	diags = diags.Append(corelinting.UnusedVariables(ctx, unusedVariables(graph)))
	diags = diags.Append(corelinting.UnusedLocal(ctx, unusedLocals(graph)))
	walker, walkDiags := c.walk(ctx, graph, walkValidate, &graphWalkOpts{
		Config:                  config,
		ProviderFunctionTracker: providerFunctionTracker,
	})
	diags = diags.Append(walker.NonFatalDiagnostics)
	diags = diags.Append(walkDiags)
	if walkDiags.HasErrors() {
		return diags
	}

	return diags
}

// unusedLocals returns a iter.Seq that will provide all the configs.Local objects for the
// locals that are detected as being unused.
// The objects returned are only for the root module.
// By returning iter.Seq, the analysis is postponed and can be skipped in case the linting rule that
// needs this data is not enabled by the user.
func unusedLocals(g *Graph) iter.Seq[*configs.Local] {
	return func(yield func(*configs.Local) bool) {
		isUsed := func(n dag.Vertex) bool {
			var used bool
			for _, u := range g.UpEdges(n) {
				switch u.(type) {
				case *nodeCloseModule:
					// if this is the only reference it has, the vertex is not used
				default:
					used = true
				}
			}
			return used
		}
		for _, v := range g.Vertices() {
			switch n := v.(type) {
			case *nodeExpandLocal:
				if !n.Module.IsRoot() {
					continue
				}
				if isUsed(n) {
					continue
				}
				if !yield(n.Config) {
					return
				}
			}
		}
	}
}

// unusedVariables returns a iter.Seq that will provide all the configs.Variable objects for the
// variables that are detected as being unused.
// The objects returned are only for the root module.
// By returning iter.Seq, the analysis is postponed and can be skipped in case the linting rule that
// needs this data is not enabled by the user.
func unusedVariables(g *Graph) iter.Seq[*configs.Variable] {
	return func(yield func(variable *configs.Variable) bool) {
		isUsed := func(n dag.Vertex) bool {
			var used bool
			for _, u := range g.UpEdges(n) {
				switch u.(type) {
				case *nodeCloseModule:
					// if this is the only reference it has, the vertex is not used
				default:
					used = true
				}
			}
			return used
		}
		for _, v := range g.Vertices() {
			switch n := v.(type) {
			case *nodeVariableReference:
				if !n.Module.IsRoot() {
					continue
				}
				if isUsed(n) {
					continue
				}
				if !yield(n.Config) {
					return
				}
			}
		}
	}
}
