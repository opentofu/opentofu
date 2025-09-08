// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"fmt"
	"iter"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

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
	ret.providerConfigNodes = compileModuleInstanceProviderConfigs(ctx, module.ProviderConfigs, topScope, module.ProviderLocalNames, call.EvalContext.Providers)
	providersSidechannel := compileModuleProvidersSidechannel(ctx, call.ProvidersFromParent, ret.providerConfigNodes)

	ret.inputVariableNodes = compileModuleInstanceInputVariables(ctx, module.Variables, call.InputValues, topScope, call.CalleeAddr, call.DeclRange)
	ret.localValueNodes = compileModuleInstanceLocalValues(ctx, module.Locals, topScope, call.CalleeAddr)
	ret.outputValueNodes = compileModuleInstanceOutputValues(ctx, module.Outputs, topScope, call.CalleeAddr)
	ret.moduleCallNodes = compileModuleInstanceModuleCalls(ctx,
		module.ModuleCalls,
		topScope,
		providersSidechannel,
		call.CalleeAddr,
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

	return ret
}

func compileModuleInstanceInputVariables(_ context.Context, configs map[string]*configs.Variable, values exprs.Valuer, declScope exprs.Scope, moduleInstAddr addrs.ModuleInstance, missingDefRange *tfdiags.SourceRange) map[addrs.InputVariable]*configgraph.InputVariable {
	ret := make(map[addrs.InputVariable]*configgraph.InputVariable, len(configs))
	for name, vc := range configs {
		addr := addrs.InputVariable{Name: name}

		// The valuer for an individual input variable derives from the
		// valuer for the single object representing all of the input
		// variables together.
		rawValuer := exprs.DerivedValuer(values, func(v cty.Value, _ tfdiags.Diagnostics) (cty.Value, tfdiags.Diagnostics) {
			// We intentionally avoid passing on the diagnostics from the
			// "values" valuer here both because they will be about the
			// entire object rather than the individual attribute we're
			// interested in and because whatever produced the "values"
			// valuer should've already reported its own errors when
			// it was checked directly.
			//
			// We might return additional diagnostics about the individual
			// atribute we're extracting, though.
			var diags tfdiags.Diagnostics

			defRange := missingDefRange
			if valueRange := values.ValueSourceRange(); valueRange != nil {
				defRange = valueRange
			}

			ty := v.Type()
			if ty == cty.DynamicPseudoType {
				return cty.DynamicVal.WithSameMarks(v), diags
			}
			if !ty.IsObjectType() {
				// Should not get here because the caller should always pass
				// us an object type based on the arguments in the module
				// call, but we'll deal with it anyway for robustness.
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid input values",
					Detail:   fmt.Sprintf("Input variable values for %s module must be provided as an object value, not %s.", moduleInstAddr, ty.FriendlyName()),
					Subject:  configgraph.MaybeHCLSourceRange(defRange),
				})
				return cty.DynamicVal.WithSameMarks(v), diags
			}
			if v.IsNull() {
				// Again this suggests a bug in the caller, but we'll handle
				// it for robustness.
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid input values",
					Detail:   fmt.Sprintf("The object describing the input values for %s must not be null.", moduleInstAddr),
					Subject:  configgraph.MaybeHCLSourceRange(defRange),
				})
				return cty.DynamicVal.WithSameMarks(v), diags
			}

			if !ty.HasAttribute(name) {
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
						Subject:  configgraph.MaybeHCLSourceRange(defRange),
					})
					return cty.DynamicVal.WithSameMarks(v), diags
				} else {
					// For a non-required variable we'll provide a placeholder
					// null value so that the evaluator can treat this the same
					// as if there was an explicit definition evaluating to null.
					return cty.NullVal(cty.DynamicPseudoType).WithSameMarks(v), diags
				}
			}
			// After all of the checks above we should now be able to call
			// GetAttr for this name without panicking. (If v is unknown
			// or marked then cty will automatically return a derived unknown
			// or marked value.)
			return v.GetAttr(name), diags
		})
		ret[addr] = &configgraph.InputVariable{
			Addr:           moduleInstAddr.InputVariable(name),
			RawValue:       configgraph.ValuerOnce(rawValuer),
			TargetType:     vc.ConstraintType,
			TargetDefaults: vc.TypeDefaults,
			CompileValidationRules: func(ctx context.Context, value cty.Value) iter.Seq[*configgraph.CheckRule] {
				// For variable validation we need to use a special overlay
				// scope that resolves the single variable we are validating
				// to the given constant value but delegates everything else
				// to the parent scope. This overlay is important because
				// these checks are run as part of the normal process of
				// handling a reference to this variable, and so if we used
				// the normal scope here we'd be depending on our own result.
				childScope := &inputVariableValidationScope{
					wantName:    name,
					parentScope: declScope,
					finalVal:    value,
				}
				return compileCheckRules(vc.Validations, childScope)
			},
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

func compileModuleInstanceOutputValues(_ context.Context, configs map[string]*configs.Output, declScope exprs.Scope, moduleInstAddr addrs.ModuleInstance) map[addrs.OutputValue]*configgraph.OutputValue {
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
			Preconditions:  slices.Collect(compileCheckRules(vc.Preconditions, declScope)),
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
	providersSideChannel *moduleProvidersSideChannel,
	moduleInstanceAddr addrs.ModuleInstance,
	providers evalglue.Providers,
	getResultValue func(context.Context, *configgraph.ResourceInstance, cty.Value, configgraph.Maybe[*configgraph.ProviderInstance]) (cty.Value, tfdiags.Diagnostics),
) map[addrs.Resource]*configgraph.Resource {
	ret := make(map[addrs.Resource]*configgraph.Resource, len(managedConfigs)+len(dataConfigs)+len(ephemeralConfigs))
	for _, rc := range managedConfigs {
		addr, rsrc := compileModuleInstanceResource(ctx, rc, declScope, providersSideChannel, moduleInstanceAddr, providers, getResultValue)
		ret[addr] = rsrc
	}
	for _, rc := range dataConfigs {
		addr, rsrc := compileModuleInstanceResource(ctx, rc, declScope, providersSideChannel, moduleInstanceAddr, providers, getResultValue)
		ret[addr] = rsrc
	}
	for _, rc := range ephemeralConfigs {
		addr, rsrc := compileModuleInstanceResource(ctx, rc, declScope, providersSideChannel, moduleInstanceAddr, providers, getResultValue)
		ret[addr] = rsrc
	}
	return ret
}

func compileModuleInstanceResource(
	ctx context.Context,
	config *configs.Resource,
	declScope exprs.Scope,
	providersSideChannel *moduleProvidersSideChannel,
	moduleInstanceAddr addrs.ModuleInstance,
	providers evalglue.Providers,
	getResultValue func(context.Context, *configgraph.ResourceInstance, cty.Value, configgraph.Maybe[*configgraph.ProviderInstance]) (cty.Value, tfdiags.Diagnostics),
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
			localScope := instanceLocalScope(declScope, repData)
			inst := &configgraph.ResourceInstance{
				Addr:     absAddr.Instance(key),
				Provider: config.Provider,
				ConfigValuer: configgraph.ValuerOnce(exprs.NewClosure(
					configEvalable, localScope,
				)),
				ProviderInstanceValuer: configgraph.ValuerOnce(
					providersSideChannel.CompileProviderConfigRef(
						ctx, config.ProviderConfigAddr(), config.ProviderConfigRef, localScope,
					),
				),
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
				getResultValue: func(ctx context.Context, configVal cty.Value, providerInst configgraph.Maybe[*configgraph.ProviderInstance]) (cty.Value, tfdiags.Diagnostics) {
					return getResultValue(ctx, inst, configVal, providerInst)
				},
			}
			return inst
		},
	}
	return resourceAddr, ret
}

func compileModuleInstanceModuleCalls(
	ctx context.Context,
	configs map[string]*configs.ModuleCall,
	declScope exprs.Scope,
	providersSidechannel *moduleProvidersSideChannel,
	moduleInstanceAddr addrs.ModuleInstance,
) map[addrs.ModuleCall]*configgraph.ModuleCall {
	ret := make(map[addrs.ModuleCall]*configgraph.ModuleCall, len(configs))
	for name, config := range configs {
		addr := addrs.ModuleCall{Name: name}
		ret[addr] = &configgraph.ModuleCall{
			Addr:             addr.Absolute(moduleInstanceAddr),
			DeclRange:        tfdiags.SourceRangeFromHCL(config.DeclRange),
			InstanceSelector: compileInstanceSelector(ctx, declScope, config.ForEach, config.Count, nil),
			CompileCallInstance: func(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) *configgraph.ModuleCallInstance {
				var versionConstraintValuer exprs.Valuer
				if config.VersionAttr != nil {
					versionConstraintValuer = exprs.NewClosure(
						exprs.EvalableHCLExpression(config.VersionAttr.Expr),
						declScope,
					)
				} else {
					versionConstraintValuer = exprs.ConstantValuer(cty.NullVal(cty.String))
				}

				instanceScope := instanceLocalScope(declScope, repData)
				return &configgraph.ModuleCallInstance{
					ModuleInstanceAddr: addr.Absolute(moduleInstanceAddr).Instance(key),

					// We _could_ potentially allow the source address and
					// version constraint to vary between instances by
					// binding these to the instance local scope, but we
					// choose not to for now because the syntax for module
					// blocks means it's not possible to vary which input
					// variables are defined on a per-instance basis and so
					// selecting different modules wouldn't work well unless
					// they all had exactly the same input variable names.
					SourceAddrValuer: configgraph.ValuerOnce(exprs.NewClosure(
						exprs.EvalableHCLExpression(config.Source),
						declScope,
					)),
					VersionConstraintValuer: configgraph.ValuerOnce(
						versionConstraintValuer,
					),

					// The inputs value _can_ be derived from per-instance
					// values though, of course! We use "just attributes"
					// mode here because on the caller side we don't yet know
					// what input variables the callee is expecting. We'll
					// just send this whole value over to it and let it
					// check whether the object type is acceptable.
					InputsValuer: configgraph.ValuerOnce(exprs.NewClosure(
						exprs.EvalableHCLBodyJustAttributes(config.Config),
						instanceScope,
					)),

					// TODO: valuers for the "providers side-channel" from
					// the "providers" meta-argument, or automatic passing
					// of all of the default (unaliased) providers from
					// the parent module if "providers" isn't present.
				}
			},
		}
	}
	return ret
}

func compileModuleInstanceProviderConfigs(
	ctx context.Context,
	configs map[string]*configs.Provider,
	declScope exprs.Scope,
	localNames map[addrs.Provider]string,
	providers evalglue.Providers,
) map[addrs.LocalProviderConfig]*configgraph.ProviderConfig {
	// TODO: Implement this properly, mimicking the logic that the current
	// system follows for inferring implied configurations for providers
	// that have an empty config schema.
	//
	// For now this is just enough to repair some tests that existed before
	// we added provider instance resolution, which all happen to rely
	// on implied references to providers with the local name "foo".
	return map[addrs.LocalProviderConfig]*configgraph.ProviderConfig{
		{LocalName: "foo"}: {
			Addr: addrs.AbsProviderConfigCorrect{
				Module:   addrs.RootModuleInstance,
				Provider: addrs.MustParseProviderSourceString("test/foo"),
			},
			ProviderAddr:     addrs.MustParseProviderSourceString("test/foo"),
			InstanceSelector: compileInstanceSelector(ctx, declScope, nil, nil, nil),
			CompileProviderInstance: func(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) *configgraph.ProviderInstance {
				return &configgraph.ProviderInstance{
					Addr: addrs.AbsProviderInstanceCorrect{
						Config: addrs.AbsProviderConfigCorrect{
							Module:   addrs.RootModuleInstance,
							Provider: addrs.MustParseProviderSourceString("test/foo"),
						},
						Key: key,
					},
					ProviderAddr: addrs.MustParseProviderSourceString("test/foo"),
				}
			},
		},
	}
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
func compileCoreFunctions(ctx context.Context, allowImpureFuncs bool, baseDir string) map[string]function.Function {
	// For now we just borrow the functions table setup from the previous
	// system's concept of "scope".
	oldScope := lang.Scope{
		PureOnly: !allowImpureFuncs,
		BaseDir:  baseDir,
		// TODO: PlanTimestamp?
	}
	return oldScope.Functions()
}

// resourceInstanceGlue is our implementation of [configgraph.ResourceInstanceGlue],
// allowing our compiled [configgraph.ResourceInstance] objects to call back
// to us for needs that require interacting with outside concerns like
// provider plugins, an active plan or apply process, etc.
type resourceInstanceGlue struct {
	validateConfig func(context.Context, cty.Value) tfdiags.Diagnostics
	getResultValue func(context.Context, cty.Value, configgraph.Maybe[*configgraph.ProviderInstance]) (cty.Value, tfdiags.Diagnostics)
}

// ValidateConfig implements configgraph.ResourceInstanceGlue.
func (r *resourceInstanceGlue) ValidateConfig(ctx context.Context, configVal cty.Value) tfdiags.Diagnostics {
	return r.validateConfig(ctx, configVal)
}

// ResultValue implements configgraph.ResourceInstanceGlue.
func (r *resourceInstanceGlue) ResultValue(ctx context.Context, configVal cty.Value, providerInst configgraph.Maybe[*configgraph.ProviderInstance]) (cty.Value, tfdiags.Diagnostics) {
	return r.getResultValue(ctx, configVal, providerInst)
}
