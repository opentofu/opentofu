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
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func compileModuleInstanceResources(
	ctx context.Context,
	managedConfigs map[string]*configs.Resource,
	dataConfigs map[string]*configs.Resource,
	ephemeralConfigs map[string]*configs.Resource,
	declScope exprs.Scope,
	providersSideChannel *moduleProvidersSideChannel,
	moduleInstanceAddr addrs.ModuleInstance,
	providers evalglue.ProvidersSchema,
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
	providers evalglue.ProvidersSchema,
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
				getResultValue: func(ctx context.Context, configVal cty.Value, providerInst configgraph.Maybe[*configgraph.ProviderInstance]) (cty.Value, tfdiags.Diagnostics) {
					return getResultValue(ctx, inst, configVal, providerInst)
				},
			}
			return inst
		},
	}
	return resourceAddr, ret
}

// resourceInstanceGlue is our implementation of [configgraph.ResourceInstanceGlue],
// allowing our compiled [configgraph.ResourceInstance] objects to call back
// to us for needs that require interacting with outside concerns like
// provider plugins, an active plan or apply process, etc.
type resourceInstanceGlue struct {
	getResultValue func(context.Context, cty.Value, configgraph.Maybe[*configgraph.ProviderInstance]) (cty.Value, tfdiags.Diagnostics)
}

// ResultValue implements configgraph.ResourceInstanceGlue.
func (r *resourceInstanceGlue) ResultValue(ctx context.Context, configVal cty.Value, providerInst configgraph.Maybe[*configgraph.ProviderInstance]) (cty.Value, tfdiags.Diagnostics) {
	return r.getResultValue(ctx, configVal, providerInst)
}
