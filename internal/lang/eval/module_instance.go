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
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

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
	evalCtx *EvalContext,
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
	ret.InputVariableNodes = compileModuleInstanceInputVariables(ctx, module.Variables, call.inputValues, ret, call.declRange)
	ret.LocalValueNodes = compileModuleInstanceLocalValues(ctx, module.Locals, ret)
	ret.OutputValueNodes = compileModuleInstanceOutputValues(ctx, module.Outputs, ret)
	ret.ResourceNodes = compileModuleInstanceResources(ctx, module.ManagedResources, module.DataResources, module.EphemeralResources, ret, call.calleeAddr, evalCtx.Providers)

	return ret
}

func compileModuleInstanceInputVariables(_ context.Context, configs map[string]*configs.Variable, values map[addrs.InputVariable]exprs.Valuer, declScope exprs.Scope, missingDefRange *tfdiags.SourceRange) map[addrs.InputVariable]*configgraph.InputVariable {
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
			DeclName:        name,
			RawValue:        configgraph.ValuerOnce(rawValue),
			TargetType:      vc.ConstraintType,
			TargetDefaults:  vc.TypeDefaults,
			ValidationRules: compileCheckRules(vc.Validations, declScope, vc.Ephemeral),
		}
	}
	return ret
}

func compileModuleInstanceLocalValues(_ context.Context, configs map[string]*configs.Local, declScope exprs.Scope) map[addrs.LocalValue]*configgraph.LocalValue {
	ret := make(map[addrs.LocalValue]*configgraph.LocalValue, len(configs))
	for name, vc := range configs {
		addr := addrs.LocalValue{Name: name}
		value := configgraph.ValuerOnce(exprs.NewClosure(
			exprs.EvalableHCLExpression(vc.Expr),
			declScope,
		))
		ret[addr] = &configgraph.LocalValue{
			RawValue: value,
		}
	}
	return ret
}

func compileModuleInstanceOutputValues(_ context.Context, configs map[string]*configs.Output, declScope *configgraph.ModuleInstance) map[addrs.OutputValue]*configgraph.OutputValue {
	ret := make(map[addrs.OutputValue]*configgraph.OutputValue, len(configs))
	for name, vc := range configs {
		addr := addrs.OutputValue{Name: name}
		value := configgraph.ValuerOnce(exprs.NewClosure(
			exprs.EvalableHCLExpression(vc.Expr),
			declScope,
		))
		ret[addr] = &configgraph.OutputValue{
			DeclName: name,
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
) map[addrs.Resource]*configgraph.Resource {
	ret := make(map[addrs.Resource]*configgraph.Resource, len(managedConfigs)+len(dataConfigs)+len(ephemeralConfigs))
	for _, rc := range managedConfigs {
		addr, rsrc := compileModuleInstanceResource(ctx, rc, declScope, moduleInstanceAddr, providers)
		ret[addr] = rsrc
	}
	for _, rc := range dataConfigs {
		addr, rsrc := compileModuleInstanceResource(ctx, rc, declScope, moduleInstanceAddr, providers)
		ret[addr] = rsrc
	}
	for _, rc := range ephemeralConfigs {
		addr, rsrc := compileModuleInstanceResource(ctx, rc, declScope, moduleInstanceAddr, providers)
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
		configEvalable = exprs.EvalableHCLBody(config.Config, spec)
	}

	ret := &configgraph.Resource{
		Addr:           absAddr,
		ConfigEvalable: configEvalable,
		ParentScope:    declScope,
		// TODO: Everything else
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
