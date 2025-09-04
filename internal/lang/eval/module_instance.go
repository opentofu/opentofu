// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// The functions and types in this file are concerned with "compiling" the
// representation of modules in [configs] into the representation used by
// [configgraph].
//
// The design idea here is that [configgraph] works in terms of OpenTofu's
// langage concepts and [cty.Value] but is not aware of the current physical
// syntax or how [configs] represents it, so that in theory we could have
// different translations to those concepts for different editions of the
// language in future. This is unlike traditional "package tofu" where
// everything is very tightly coupled to [configs], making it hard to evolve
// (or replace) that package over time.
//
// The code in here could probably move into another package under the
// "internal" directory rather than being inline here, since this logic
// is thematically separate from the "config eval" functionality provided
// by the rest of this package, but it's here for now just to avoid
// overthinking too much while this is still evolving.

type moduleInstanceCall struct {
	// calleeAddr is the absolute address of this module instance that should
	// be used as the basis for calculating addresses of resources within
	// and beneath it.
	calleeAddr addrs.ModuleInstance

	// declRange is the source location of the header of the module block
	// that is making this call, or some similar config construct that's
	// acting like a module call.
	//
	// This should be nil for calls that are caused by something other than
	// configuration, such as a top-level call to a root module caused by
	// running an OpenTofu CLI command.
	declRange *tfdiags.SourceRange

	// inputValues describes how to build the values for the input variables
	// for this instance of the module.
	//
	// For a call caused by a "module" block in a parent module, these would
	// be closures binding the expressions written in the module block to
	// the scope of the module block. The scope of the module block should
	// include the each.key/each.value/count.index symbols initialized as
	// appropriate for this specific instance of the module call. It's
	// the caller of [compileModuleInstance]'s responsibility to set these
	// up correctly so that the child module can be compiled with no direct
	// awareness of where it's being called from.
	inputValues map[addrs.InputVariable]exprs.Valuer

	// TODO: provider instances from the "providers" argument in the
	// calling module, once we have enough of this implemented for that
	// to be useful. Will need to straighten out the address types
	// for provider configs and instances in package addrs first so
	// that we finally have a proper address type for a provider
	// instance with an instance key.

	// evaluationGlue is the [evaluationGlue] implementation to use when
	// the evaluator needs information from outside of the configuration.
	//
	// All module instances belonging to a single configuration tree should
	// typically share the same evaluationGlue.
	evaluationGlue evaluationGlue

	// evalContext is the [EvalContext] to use to interact with the context.
	//
	// Compared to evaluationGlue, evalContext deals with concerns that
	// are typically held constant throughout sequential validate, plan, and
	// apply phases, whereas evaluationGlue is where we deal with behaviors
	// that need to vary between phases.
	evalContext *EvalContext
}

// compileModuleInstance is the main entry point for binding a module
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
func compileModuleInstance(
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

	call *moduleInstanceCall,
) *configgraph.ModuleInstance {
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
	// [configgraph.ModuleInstance] since many of them only make sense to
	// call everything has been completely assembled.
	ret := &configgraph.ModuleInstance{
		ModuleSourceAddr: moduleSourceAddr,
		CallDeclRange:    call.declRange,
	}
	ret.InputVariableNodes = compileModuleInstanceInputVariables(ctx, module.Variables, call.inputValues, ret, call.calleeAddr, call.declRange)
	ret.LocalValueNodes = compileModuleInstanceLocalValues(ctx, module.Locals, ret, call.calleeAddr)
	ret.OutputValueNodes = compileModuleInstanceOutputValues(ctx, module.Outputs, ret, call.calleeAddr)
	ret.ResourceNodes = compileModuleInstanceResources(ctx, module.ManagedResources, module.DataResources, module.EphemeralResources, ret, call.calleeAddr, call.evalContext.Providers, call.evaluationGlue.ResourceInstanceValue)

	return ret
}

func compileModuleInstanceInputVariables(_ context.Context, configs map[string]*configs.Variable, values map[addrs.InputVariable]exprs.Valuer, declScope exprs.Scope, moduleInstAddr addrs.ModuleInstance, missingDefRange *tfdiags.SourceRange) map[addrs.InputVariable]*configgraph.InputVariable {
	ret := make(map[addrs.InputVariable]*configgraph.InputVariable, len(configs))
	for name, vc := range configs {
		addr := addrs.InputVariable{Name: name}

		rawValue, ok := values[addr]
		if !ok {
			diagRange := vc.DeclRange
			if missingDefRange != nil {
				// better to blame the definition site than the declaration
				// site if we have enough information to do that.
				diagRange = missingDefRange.ToHCL()
			}
			if vc.Required() {
				// We don't actually _need_ to handle an error here because
				// the final evaluation of the variables must deal with the
				// possibility of the final value being null anyway, but
				// by handling this here we can produce a more helpful error
				// message that talks about the definition being statically
				// absent instead of dynamically null.
				var diags tfdiags.Diagnostics
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Missing definition for required input variable",
					Detail:   fmt.Sprintf("Input variable %q is required, and so it must be provided as an argument to this module.", name),
					Subject:  &diagRange,
				})
				rawValue = exprs.ForcedErrorValuer(diags)
			} else {
				// For a non-required variable we'll provide a placeholder
				// null value so that the evaluator can treat this the same
				// as if there was an explicit definition evaluating to null.
				rawValue = exprs.ConstantValuerWithSourceRange(
					cty.NullVal(vc.Type),
					tfdiags.SourceRangeFromHCL(diagRange),
				)
			}
		}
		ret[addr] = &configgraph.InputVariable{
			Addr:            moduleInstAddr.InputVariable(name),
			RawValue:        configgraph.ValuerOnce(rawValue),
			TargetType:      vc.ConstraintType,
			TargetDefaults:  vc.TypeDefaults,
			ValidationRules: compileCheckRules(vc.Validations, declScope, vc.Ephemeral),
		}
	}
	return ret
}

func compileModuleInstanceLocalValues(_ context.Context, configs map[string]*configs.Local, declScope exprs.Scope, moduleInstAddr addrs.ModuleInstance) map[addrs.LocalValue]*configgraph.LocalValue {
	ret := make(map[addrs.LocalValue]*configgraph.LocalValue, len(configs))
	for name, vc := range configs {
		addr := addrs.LocalValue{Name: name}
		value := configgraph.ValuerOnce(exprs.NewClosure(
			exprs.EvalableHCLExpression(vc.Expr),
			declScope,
		))
		ret[addr] = &configgraph.LocalValue{
			Addr:     moduleInstAddr.LocalValue(name),
			RawValue: value,
		}
	}
	return ret
}

func compileModuleInstanceOutputValues(_ context.Context, configs map[string]*configs.Output, declScope *configgraph.ModuleInstance, moduleInstAddr addrs.ModuleInstance) map[addrs.OutputValue]*configgraph.OutputValue {
	ret := make(map[addrs.OutputValue]*configgraph.OutputValue, len(configs))
	for name, vc := range configs {
		addr := addrs.OutputValue{Name: name}
		value := configgraph.ValuerOnce(exprs.NewClosure(
			exprs.EvalableHCLExpression(vc.Expr),
			declScope,
		))
		ret[addr] = &configgraph.OutputValue{
			Addr:     moduleInstAddr.OutputValue(name),
			RawValue: value,

			// Our current language doesn't allow specifying a type constraint
			// for an output value, so these are always the most liberal
			// possible constraint. Making these customizable could be part
			// of a solution to:
			//     https://github.com/opentofu/opentofu/issues/2831
			TargetType:     cty.DynamicPseudoType,
			TargetDefaults: nil,

			ForceSensitive: vc.Sensitive,
			ForceEphemeral: vc.Ephemeral,
			Preconditions:  compileCheckRules(vc.Preconditions, declScope, vc.Ephemeral),
		}
	}
	return ret
}

func compileModuleInstanceResources(
	ctx context.Context,
	managedConfigs map[string]*configs.Resource,
	dataConfigs map[string]*configs.Resource,
	ephemeralConfigs map[string]*configs.Resource,
	declScope exprs.Scope,
	moduleInstanceAddr addrs.ModuleInstance,
	providers Providers,
	getResultValue func(context.Context, *configgraph.ResourceInstance, cty.Value) (cty.Value, tfdiags.Diagnostics),
) map[addrs.Resource]*configgraph.Resource {
	ret := make(map[addrs.Resource]*configgraph.Resource, len(managedConfigs)+len(dataConfigs)+len(ephemeralConfigs))
	for _, rc := range managedConfigs {
		addr, rsrc := compileModuleInstanceResource(ctx, rc, declScope, moduleInstanceAddr, providers, getResultValue)
		ret[addr] = rsrc
	}
	for _, rc := range dataConfigs {
		addr, rsrc := compileModuleInstanceResource(ctx, rc, declScope, moduleInstanceAddr, providers, getResultValue)
		ret[addr] = rsrc
	}
	for _, rc := range ephemeralConfigs {
		addr, rsrc := compileModuleInstanceResource(ctx, rc, declScope, moduleInstanceAddr, providers, getResultValue)
		ret[addr] = rsrc
	}
	return ret
}

func compileModuleInstanceResource(
	ctx context.Context,
	config *configs.Resource,
	declScope exprs.Scope,
	moduleInstanceAddr addrs.ModuleInstance,
	providers Providers,
	getResultValue func(context.Context, *configgraph.ResourceInstance, cty.Value) (cty.Value, tfdiags.Diagnostics),
) (addrs.Resource, *configgraph.Resource) {
	resourceAddr := config.Addr()
	absAddr := moduleInstanceAddr.Resource(resourceAddr.Mode, resourceAddr.Type, resourceAddr.Name)

	var configEvalable exprs.Evalable
	resourceTypeSchema, diags := providers.ResourceTypeSchema(ctx,
		config.Provider,
		resourceAddr.Mode,
		resourceAddr.Type,
	)
	if diags.HasErrors() {
		configEvalable = exprs.ForcedErrorEvalable(diags, tfdiags.SourceRangeFromHCL(config.DeclRange))
	} else {
		spec := resourceTypeSchema.Block.DecoderSpec()
		configEvalable = exprs.EvalableHCLBodyWithDynamicBlocks(config.Config, spec)
	}

	ret := &configgraph.Resource{
		Addr:      absAddr,
		DeclRange: tfdiags.SourceRangeFromHCL(config.DeclRange),

		// Our instance selector depends on which of the repetition metaarguments
		// are set, if any. We assume that package configs allows at most one
		// of these to be set for each resource config.
		InstanceSelector: compileInstanceSelector(ctx, declScope, config.ForEach, config.Count, nil),

		// The [configgraph.Resource] implementation will call back to this
		// for each child instance it discovers through [InstanceSelector],
		// allowing us to finalize all of the details for a specific instance
		// of this resource.
		CompileResourceInstance: func(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) *configgraph.ResourceInstance {
			inst := &configgraph.ResourceInstance{
				Addr:     absAddr.Instance(key),
				Provider: config.Provider,
				ConfigValuer: configgraph.ValuerOnce(exprs.NewClosure(
					configEvalable,
					configgraph.InstanceLocalScope(declScope, repData),
				)),
			}
			// Again the [ResourceInstance] implementation will call back
			// through this object so we can help it interact with the
			// appropriate provider and collect the result of whatever
			// side-effects our caller is doing with this resource instance
			// in the current phase. (The planned new state during the plan
			// phase, for example.)
			inst.Glue = &resourceInstanceGlue{
				validateConfig: func(ctx context.Context, configVal cty.Value) tfdiags.Diagnostics {
					return providers.ValidateResourceConfig(ctx, config.Provider, resourceAddr.Mode, resourceAddr.Type, configVal)
				},
				getResultValue: func(ctx context.Context, configVal cty.Value) (cty.Value, tfdiags.Diagnostics) {
					return getResultValue(ctx, inst, configVal)
				},
			}
			return inst
		},
	}
	return resourceAddr, ret
}

func compileCheckRules(configs []*configs.CheckRule, declScope exprs.Scope, ephemeralAllowed bool) []configgraph.CheckRule {
	ret := make([]configgraph.CheckRule, 0, len(configs))
	for _, config := range configs {
		ret = append(ret, configgraph.CheckRule{
			Condition:        exprs.EvalableHCLExpression(config.Condition),
			ErrorMessageRaw:  exprs.EvalableHCLExpression(config.ErrorMessage),
			ParentScope:      declScope,
			EphemeralAllowed: ephemeralAllowed,
			DeclSourceRange:  tfdiags.SourceRangeFromHCL(config.DeclRange),
		})
	}
	return ret
}

type resourceInstanceGlue struct {
	validateConfig func(context.Context, cty.Value) tfdiags.Diagnostics
	getResultValue func(context.Context, cty.Value) (cty.Value, tfdiags.Diagnostics)
}

// ValidateConfig implements configgraph.ResourceInstanceGlue.
func (r *resourceInstanceGlue) ValidateConfig(ctx context.Context, configVal cty.Value) tfdiags.Diagnostics {
	return r.validateConfig(ctx, configVal)
}

// ResultValue implements configgraph.ResourceInstanceGlue.
func (r *resourceInstanceGlue) ResultValue(ctx context.Context, configVal cty.Value) (cty.Value, tfdiags.Diagnostics) {
	return r.getResultValue(ctx, configVal)
}
