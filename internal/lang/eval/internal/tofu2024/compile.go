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
	ret.providerConfigNodes = compileModuleInstanceProviderConfigs(ctx,
		module.ProviderConfigs,
		allResourcesFromModule(module),
		topScope,
		module.ProviderRequirements.RequiredProviders,
		call.CalleeAddr,
		call.EvalContext.Providers,
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
	parentSourceAddr addrs.ModuleSource,
	moduleInstanceAddr addrs.ModuleInstance,
	externalModules evalglue.ExternalModules,
	parentCall *ModuleInstanceCall,
) map[addrs.ModuleCall]*configgraph.ModuleCall {
	ret := make(map[addrs.ModuleCall]*configgraph.ModuleCall, len(configs))
	for name, config := range configs {
		addr := addrs.ModuleCall{Name: name}
		absAddr := addr.Absolute(moduleInstanceAddr)

		var versionConstraintValuer exprs.Valuer
		if config.VersionAttr != nil {
			versionConstraintValuer = exprs.NewClosure(
				exprs.EvalableHCLExpression(config.VersionAttr.Expr),
				declScope,
			)
		} else {
			versionConstraintValuer = exprs.ConstantValuer(cty.NullVal(cty.String))
		}

		ret[addr] = &configgraph.ModuleCall{
			Addr:             addr.Absolute(moduleInstanceAddr),
			DeclRange:        tfdiags.SourceRangeFromHCL(config.DeclRange),
			ParentSourceAddr: parentSourceAddr,
			InstanceSelector: compileInstanceSelector(ctx, declScope, config.ForEach, config.Count, nil),
			SourceAddrValuer: configgraph.ValuerOnce(exprs.NewClosure(
				exprs.EvalableHCLExpression(config.Source),
				declScope,
			)),
			VersionConstraintValuer: configgraph.ValuerOnce(
				versionConstraintValuer,
			),
			ValidateSourceArguments: func(ctx context.Context, sourceArgs configgraph.ModuleSourceArguments) tfdiags.Diagnostics {
				// We'll try to use the given source address with our
				// [ExternalModules] object, and consider the arguments to be
				// valid if this succeeds.
				//
				// If the [ExternalModules] object is one that fetches the
				// requested module over the network on first request then we
				// expect that it'll cache what it fetched somewhere so that
				// a subsequent call with the same arguments will be relatively
				// cheap.
				_, diags := externalModules.ModuleConfig(ctx, sourceArgs.Source, sourceArgs.AllowedVersions, &absAddr)
				return diags
			},
			CompileCallInstance: func(ctx context.Context, sourceArgs configgraph.ModuleSourceArguments, key addrs.InstanceKey, repData instances.RepetitionData) *configgraph.ModuleCallInstance {

				// The contract for [configgraph.ModuleCall] is that it should only
				// call this function when sourceArgs is something that was previously
				// accepted by [ValidateSourceArguments]. We assume that the module
				// dependencies object is doing some sort of caching so that if
				// ValidateSourceArguments caused something to be downloaded over
				// the network then we can re-request that same object cheaply here.
				mod, diags := externalModules.ModuleConfig(ctx, sourceArgs.Source, sourceArgs.AllowedVersions, &absAddr)
				if diags.HasErrors() {
					// We should not typically find errors here because we would've
					// already tried this in ValidateSourceArguments above, but
					// we _do_ encounter problems here then we'll return a stubby
					// object that just returns whatever diagnostics we found as
					// soon as it tries to evaluate its inputs.
					return &configgraph.ModuleCallInstance{
						ModuleInstanceAddr: addr.Absolute(moduleInstanceAddr).Instance(key),
						InputsValuer:       configgraph.ValuerOnce(exprs.ForcedErrorValuer(diags)),
						Glue: &moduleCallInstanceGlue{
							validateInputs: func(ctx context.Context, v cty.Value) tfdiags.Diagnostics {
								return diags
							},
							getOutputsValue: func(ctx context.Context, v cty.Value) (cty.Value, tfdiags.Diagnostics) {
								return cty.DynamicVal, diags
							},
						},
					}
				}

				instanceScope := instanceLocalScope(declScope, repData)
				return &configgraph.ModuleCallInstance{
					ModuleInstanceAddr: addr.Absolute(moduleInstanceAddr).Instance(key),

					InputsValuer: configgraph.ValuerOnce(exprs.NewClosure(
						exprs.EvalableHCLBodyJustAttributes(config.Config),
						instanceScope,
					)),
					Glue: &moduleCallInstanceGlue{
						validateInputs: func(ctx context.Context, v cty.Value) tfdiags.Diagnostics {
							return mod.ValidateModuleInputs(ctx, v)
						},
						getOutputsValue: func(ctx context.Context, v cty.Value) (cty.Value, tfdiags.Diagnostics) {
							modInst, diags := mod.CompileModuleInstance(ctx, &evalglue.ModuleCall{
								InputValues:          exprs.ConstantValuer(v),
								AllowImpureFunctions: parentCall.AllowImpureFunctions,
								EvalContext:          parentCall.EvalContext,
								EvaluationGlue:       parentCall.EvaluationGlue,
							})
							if diags.HasErrors() {
								return cty.DynamicVal, diags
							}
							ret, moreDiags := modInst.ResultValuer(ctx).Value(ctx)
							diags = diags.Append(moreDiags)
							return ret, diags
						},
					},
				}
			},
		}
	}
	return ret
}

func compileModuleInstanceProviderConfigs(
	ctx context.Context,
	configs map[string]*configs.Provider,
	allResources iter.Seq[*configs.Resource],
	declScope exprs.Scope,
	reqdProviders map[string]*configs.RequiredProvider,
	moduleInstanceAddr addrs.ModuleInstance,
	providers evalglue.Providers,
) map[addrs.LocalProviderConfig]*configgraph.ProviderConfig {
	// FIXME: The following is just enough to make simple examples work, but
	// doesn't closely match the rather complicated way that OpenTofu has
	// traditionally dealt with provider configurations inheriting between
	// modules, etc. If we decide to move forward with this then we should
	// study this and the old behavior carefully and make sure they achieve
	// equivalent results. (But note that this function is only part of the
	// process: compileModuleProvidersSidechannel also deals with part of
	// this problem.)

	ret := make(map[addrs.LocalProviderConfig]*configgraph.ProviderConfig, len(configs))

	// First we'll deal with the explicitly-declared ones.
	for _, config := range configs {
		providerAddr := addrs.NewDefaultProvider(config.Name)
		if reqd, ok := reqdProviders[config.Name]; ok {
			providerAddr = reqd.Type
		}
		localAddr := addrs.LocalProviderConfig{
			LocalName: config.Name,
			Alias:     config.Alias,
		}

		var configEvalable exprs.Evalable
		configSchema, diags := providers.ProviderConfigSchema(ctx, providerAddr)
		if diags.HasErrors() {
			configEvalable = exprs.ForcedErrorEvalable(diags, tfdiags.SourceRangeFromHCL(config.DeclRange))
		} else {
			spec := configSchema.Block.DecoderSpec()
			configEvalable = exprs.EvalableHCLBodyWithDynamicBlocks(config.Config, spec)
		}

		ret[localAddr] = &configgraph.ProviderConfig{
			Addr: addrs.AbsProviderConfigCorrect{
				Module:   moduleInstanceAddr,
				Provider: providerAddr,
			},
			ProviderAddr:     providerAddr,
			InstanceSelector: compileInstanceSelector(ctx, declScope, config.ForEach, nil, nil),
			CompileProviderInstance: func(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) *configgraph.ProviderInstance {
				instanceScope := instanceLocalScope(declScope, repData)
				return &configgraph.ProviderInstance{
					Addr: addrs.AbsProviderInstanceCorrect{
						Config: addrs.AbsProviderConfigCorrect{
							Module:   addrs.RootModuleInstance,
							Provider: providerAddr,
						},
						Key: key,
					},
					ProviderAddr: providerAddr,
					ConfigValuer: configgraph.ValuerOnce(
						exprs.NewClosure(configEvalable, instanceScope),
					),
					ValidateConfig: func(ctx context.Context, v cty.Value) tfdiags.Diagnostics {
						return providers.ValidateProviderConfig(ctx, providerAddr, v)
					},
				}
			},
		}
	}

	// Now we'll add the implied ones with empty configs, but only if we're
	// in the root module because implied configs are not supposed to appear
	// in other modules.
	if moduleInstanceAddr.IsRoot() {
		for resourceConfig := range allResources {
			localAddr := resourceConfig.ProviderConfigAddr()
			if _, ok := ret[localAddr]; ok {
				continue // we already have an instance for this local address
			}
			providerAddr := addrs.NewDefaultProvider(localAddr.LocalName)
			if reqd, ok := reqdProviders[localAddr.LocalName]; ok {
				providerAddr = reqd.Type
			}

			// For these implied ones there isn't actually any real provider
			// config and so we just pretend that there was a block with
			// an empty body. If the provider schema includes any required
			// arguments this will then fail due to the fake empty body not
			// conforming to the schema.
			var configEvalable exprs.Evalable
			configSchema, diags := providers.ProviderConfigSchema(ctx, providerAddr)
			if diags.HasErrors() {
				// We don't really have any good source location to blame for
				// problems here, so we'll just arbitrarily blame one of the
				// resources that refers to the provider for now. The error
				// reporting for these situations being poor has been a classic
				// problem even in the previous implementation so hopefully we
				// can think of a better idea for this at some point...
				configEvalable = exprs.ForcedErrorEvalable(diags, tfdiags.SourceRangeFromHCL(resourceConfig.DeclRange))
			} else {
				spec := configSchema.Block.DecoderSpec()
				configEvalable = exprs.EvalableHCLBodyWithDynamicBlocks(hcl.EmptyBody(), spec)
			}

			ret[localAddr] = &configgraph.ProviderConfig{
				Addr: addrs.AbsProviderConfigCorrect{
					Module:   moduleInstanceAddr,
					Provider: providerAddr,
				},
				ProviderAddr:     providerAddr,
				InstanceSelector: compileInstanceSelector(ctx, declScope, nil, nil, nil),
				CompileProviderInstance: func(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) *configgraph.ProviderInstance {
					instanceScope := instanceLocalScope(declScope, repData)
					return &configgraph.ProviderInstance{
						Addr: addrs.AbsProviderInstanceCorrect{
							Config: addrs.AbsProviderConfigCorrect{
								Module:   addrs.RootModuleInstance,
								Provider: providerAddr,
							},
							Key: key,
						},
						ProviderAddr: providerAddr,
						ConfigValuer: configgraph.ValuerOnce(
							exprs.NewClosure(configEvalable, instanceScope),
						),
						ValidateConfig: func(ctx context.Context, v cty.Value) tfdiags.Diagnostics {
							return providers.ValidateProviderConfig(ctx, providerAddr, v)
						},
					}
				},
			}
		}
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

// allResourcesFromModule is a helper to collect all of the resources from
// a module configuration regardless of mode, since the underlying
// representation uses a separate map per mode.
func allResourcesFromModule(mod *configs.Module) iter.Seq[*configs.Resource] {
	return func(yield func(*configs.Resource) bool) {
		for _, r := range mod.ManagedResources {
			if !yield(r) {
				return
			}
		}
		for _, r := range mod.DataResources {
			if !yield(r) {
				return
			}
		}
		for _, r := range mod.EphemeralResources {
			if !yield(r) {
				return
			}
		}
	}
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

type moduleCallInstanceGlue struct {
	validateInputs  func(context.Context, cty.Value) tfdiags.Diagnostics
	getOutputsValue func(context.Context, cty.Value) (cty.Value, tfdiags.Diagnostics)
}

func (g *moduleCallInstanceGlue) ValidateInputs(ctx context.Context, inputsVal cty.Value) tfdiags.Diagnostics {
	return g.validateInputs(ctx, inputsVal)
}

func (g *moduleCallInstanceGlue) OutputsValue(ctx context.Context, inputsVal cty.Value) (cty.Value, tfdiags.Diagnostics) {
	return g.getOutputsValue(ctx, inputsVal)
}
