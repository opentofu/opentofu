// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

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
				calleeAddr := moduleInstanceAddr.Child(addr.Name, key)

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
					inst := &configgraph.ModuleCallInstance{
						ModuleInstanceAddr: addr.Absolute(moduleInstanceAddr).Instance(key),
						InputsValuer:       configgraph.ValuerOnce(exprs.ForcedErrorValuer(diags)),
					}
					inst.Glue = &moduleCallInstanceGlue{
						callInstNode: inst,
						validateInputs: func(ctx context.Context, v cty.Value) tfdiags.Diagnostics {
							return diags
						},
						compileChild: func(ctx context.Context, v cty.Value) (configgraph.Maybe[evalglue.CompiledModuleInstance], tfdiags.Diagnostics) {
							return nil, nil
						},
					}
					return inst
				}

				instanceScope := instanceLocalScope(declScope, repData)
				// TODO: The following is kinda tangled and messy, with a
				// mutual dependency between the [configgraph.ModuleCallInstance]
				// and the [moduleCallInstanceGlue] object it uses to call
				// back out to us. This achieves the intended separation of
				// concerns between compiler and configraph but hopefully we
				// can find a simpler way to get there without so much
				// back-and-forth.
				inst := &configgraph.ModuleCallInstance{
					ModuleInstanceAddr: addr.Absolute(moduleInstanceAddr).Instance(key),

					InputsValuer: configgraph.ValuerOnce(exprs.NewClosure(
						exprs.EvalableHCLBodyJustAttributes(config.Config),
						instanceScope,
					)),
				}
				inst.Glue = &moduleCallInstanceGlue{
					callInstNode: inst,
					validateInputs: func(ctx context.Context, v cty.Value) tfdiags.Diagnostics {
						return mod.ValidateModuleInputs(ctx, v)
					},
					compileChild: func(ctx context.Context, v cty.Value) (configgraph.Maybe[evalglue.CompiledModuleInstance], tfdiags.Diagnostics) {
						modInst, diags := mod.CompileModuleInstance(ctx, calleeAddr, &evalglue.ModuleCall{
							InputValues:          exprs.ConstantValuer(v),
							AllowImpureFunctions: parentCall.AllowImpureFunctions,
							EvalContext:          parentCall.EvalContext,
							EvaluationGlue:       parentCall.EvaluationGlue,
						})
						if diags.HasErrors() {
							return nil, diags
						}
						return configgraph.Known(modInst), diags
					},
				}
				return inst
			},
		}
	}
	return ret
}

type moduleCallInstanceGlue struct {
	callInstNode *configgraph.ModuleCallInstance

	validateInputs func(context.Context, cty.Value) tfdiags.Diagnostics
	compileChild   func(context.Context, cty.Value) (configgraph.Maybe[evalglue.CompiledModuleInstance], tfdiags.Diagnostics)

	// FIXME: This isn't exposed in the tree of AnnounceAllGraphevalRequests
	// method calls we use to collect up user-friendly names for all of our
	// workgraph requests, so if a self-reference error occurs across this
	// boundary there will be an unnamed item in the resulting error message.
	compiledChild grapheval.Once[configgraph.Maybe[evalglue.CompiledModuleInstance]]
}

func (g *moduleCallInstanceGlue) ValidateInputs(ctx context.Context, inputsVal cty.Value) tfdiags.Diagnostics {
	return g.validateInputs(ctx, inputsVal)
}

func (g *moduleCallInstanceGlue) OutputsValue(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	// This function MUST pass through the diags returned from
	// compiledModuleInstance, so that they can be returned as part of deciding
	// the final value of the associated [configgraph.ModuleCallInstance].
	maybeCompiled, diags := g.compiledModuleInstance(ctx)
	compiled, ok := configgraph.GetKnown(maybeCompiled)
	if !ok {
		return exprs.AsEvalError(cty.DynamicVal), diags
	}
	ret, moreDiags := compiled.ResultValuer(ctx).Value(ctx)
	diags = diags.Append(moreDiags)
	return ret, diags
}

// compiledModuleInstance is the internal wiring step of compiling the child
// module instance based on the inputs value decided by the callNode.
//
// We also use this directly in [CompiledModuleInstance]'s implementation
// of [evalglue.CompiledModuleInstance] to give the caller direct access to the
// child module instance objects for recursive tree walks.
func (g *moduleCallInstanceGlue) compiledModuleInstance(ctx context.Context) (configgraph.Maybe[evalglue.CompiledModuleInstance], tfdiags.Diagnostics) {
	return g.compiledChild.Do(ctx, func(ctx context.Context) (configgraph.Maybe[evalglue.CompiledModuleInstance], tfdiags.Diagnostics) {
		configVal, diags := g.callInstNode.InputsValue(ctx)
		if !configVal.IsKnown() {
			return nil, diags
		}
		ret, moreDiags := g.compileChild(ctx, configVal)
		diags = diags.Append(moreDiags)
		return ret, diags
	})
}
