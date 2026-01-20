// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"iter"

	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// CompileModuleInstance is the main entry point for binding a module
// configuration to information from an instance of a module call and
// producing a [configgraph.ModuleInstance] representation of the
// resulting module instance, ready for continued evaluation.
//
// For those coming to this familiar with the previous language runtime
// implementation in "package tofu": this is _roughly_ analogous to the
// graph building process but is focused only on the configuration of
// a single module (no state, no other modules) and is written as much
// as possible as straightforward linear code, with inversion of control
// techniques only where it's useful to separate concerns.
func CompileModuleInstance(
	ctx context.Context,
	module *configs.Module,

	// FIXME: This is a separate argument for now because in current
	// "package configs" this is treated as a property of the [configs.Config]
	// instead of the [configs.Module], but we're intentionally not using
	// [configs.Config] here because this new design assembles the module tree
	// gradually during evaluation rather than up front during loading.
	//
	// If we decide to take this direction we should track the source
	// address as a field of [configs.Module] so that we don't need this
	// extra argument.
	moduleSourceAddr addrs.ModuleSource,

	call *ModuleInstanceCall,
) *CompiledModuleInstance {
	// -----------------------------------------------------------------------
	// This intentionally has no direct error return path, because:
	// 1. The code that builds *configs.Module should already have reported
	//    any "static" problems like syntax errors and hard structural
	//    problems and thus prevented us from even reaching this call if
	//    any were present.
	// 2. This "compiling" step is mainly about wiring things together in
	//    preparation for evaluation rather than actually evaluating, and so
	//    _dynamic_ problems will be handled during the subsequent evaluation
	//    step rather than during this compilation process.
	//
	// If the work performed by this function _does_ discover something that's
	// invalid enough that it's impossible to construct valid evaluation
	// objects, then use mechanisms like [exprs.ForceErrorValuer] to arrange
	// for predefined error diagnostics to be discovered during evaluation
	// instead of returning them directly from here.
	// -----------------------------------------------------------------------

	// We'll build this object up gradually because what we're ultimately going
	// to return is an implied graph of the relationships between everything
	// declared in this module, represented either directly by pointers or
	// indirectly through expressions, and so for the remainder of this
	// function we need to be careful in how we interact with the methods of
	// [CompiledModuleInstance] since many of them only make sense to call
	// after everything has been completely assembled.
	ret := &CompiledModuleInstance{
		moduleInstanceNode: &configgraph.ModuleInstance{
			Addr:          call.CalleeAddr,
			CallDeclRange: call.DeclRange,
		},
	}

	// topScope is the top-level scope that defines what all normal expressions
	// within the module can refer to, such as the top-level "var" and "local"
	// symbols and all of the available functions.
	topScope := &moduleInstanceScope{
		inst:          ret,
		coreFunctions: compileCoreFunctions(ctx, call.AllowImpureFunctions, call.EvalContext.RootModuleDir),
	}

	// We have some shims in here to deal with the unusual way the existing
	// OpenTofu language deals with references to provider instances, since
	// [configgraph] is designed to support treating them as "normal" values
	// in future but we want to keep existing modules working for now.
	ret.providerConfigNodes = compileModuleInstanceProviderConfigs(ctx,
		module.ProviderConfigs,
		allResourcesFromModule(module),
		topScope,
		module.ProviderRequirements.RequiredProviders,
		call.CalleeAddr,
		call.EvalContext.Providers,
		call.EvaluationGlue.ValidateProviderConfig,
	)
	providersSidechannel := compileModuleProvidersSidechannel(ctx, call.ProvidersFromParent, ret.providerConfigNodes)

	ret.inputVariableNodes = compileModuleInstanceInputVariables(ctx, module.Variables, call.InputValues, topScope, call.CalleeAddr, call.DeclRange)
	ret.localValueNodes = compileModuleInstanceLocalValues(ctx, module.Locals, topScope, call.CalleeAddr)
	ret.outputValueNodes = compileModuleInstanceOutputValues(ctx, module.Outputs, topScope, call.CalleeAddr)
	ret.moduleCallNodes = compileModuleInstanceModuleCalls(ctx,
		module.ModuleCalls,
		topScope,
		providersSidechannel,
		moduleSourceAddr,
		call.CalleeAddr,
		call.EvalContext.Modules,
		call,
	)
	ret.resourceNodes = compileModuleInstanceResources(ctx,
		module.ManagedResources,
		module.DataResources,
		module.EphemeralResources,
		topScope,
		providersSidechannel,
		call.CalleeAddr,
		call.EvalContext.Providers,
		call.EvaluationGlue.ResourceInstanceValue,
	)

	// Now that we've assembled all of the innards of the module instance,
	// we'll wire the output values up to the top-level module instance
	// node so that it can produce the overall result object for this module
	// instance.
	ret.moduleInstanceNode.OutputValuers = make(map[addrs.OutputValue]*configgraph.OnceValuer, len(ret.outputValueNodes))
	for addr, node := range ret.outputValueNodes {
		ret.moduleInstanceNode.OutputValuers[addr] = configgraph.ValuerOnce(node)
	}

	return ret
}

func compileCheckRules(configs []*configs.CheckRule, evalScope exprs.Scope) iter.Seq[*configgraph.CheckRule] {
	// TODO: Maybe we need to allow the caller to impose additional constraints
	// on the result of the ConditionValuer here, such as disallowing the
	// use of ephemeral values outside of ephemeral resource
	// preconditions/postconditions. If so, perhaps we'd take an additional
	// argument for an optional callback function that takes the result of
	// the condition expression and can return additional diagnostics that
	// make sense for the specific context where the check rules are being used.
	return func(yield func(*configgraph.CheckRule) bool) {
		for _, config := range configs {
			compiled := &configgraph.CheckRule{
				ConditionValuer: exprs.NewClosure(
					exprs.EvalableHCLExpression(config.Condition),
					evalScope,
				),
				ErrorMessageValuer: exprs.NewClosure(
					exprs.EvalableHCLExpression(config.ErrorMessage),
					evalScope,
				),
				DeclSourceRange: tfdiags.SourceRangeFromHCL(config.DeclRange),
			}
			if !yield(compiled) {
				return
			}
		}
	}
}

// compileCoreFunctions prepares the table of core functions for inclusion in
// a module instance scope.
func compileCoreFunctions(_ context.Context, allowImpureFuncs bool, baseDir string) map[string]function.Function {
	// For now we just borrow the functions table setup from the previous
	// system's concept of "scope".
	oldScope := lang.Scope{
		PureOnly: !allowImpureFuncs,
		BaseDir:  baseDir,
		// TODO: PlanTimestamp?
	}
	return oldScope.Functions()
}
