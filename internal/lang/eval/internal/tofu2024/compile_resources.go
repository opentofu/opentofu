// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"fmt"
	"maps"

	"github.com/hashicorp/hcl/v2"
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
	moduleProviders configgraph.CompileProviderConfigRef,
	moduleInstanceAddr addrs.ModuleInstance,
	providers evalglue.ProvidersSchema,
	getResultValue func(context.Context, *configgraph.ResourceInstance, cty.Value, configgraph.Maybe[*configgraph.ProviderInstance], addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics),
) map[addrs.Resource]*configgraph.Resource {
	ret := make(map[addrs.Resource]*configgraph.Resource, len(managedConfigs)+len(dataConfigs)+len(ephemeralConfigs))
	for _, rc := range managedConfigs {
		addr, rsrc := compileModuleInstanceResource(ctx, rc, declScope, moduleProviders, moduleInstanceAddr, providers, getResultValue)
		ret[addr] = rsrc
	}
	for _, rc := range dataConfigs {
		addr, rsrc := compileModuleInstanceResource(ctx, rc, declScope, moduleProviders, moduleInstanceAddr, providers, getResultValue)
		ret[addr] = rsrc
	}
	for _, rc := range ephemeralConfigs {
		addr, rsrc := compileModuleInstanceResource(ctx, rc, declScope, moduleProviders, moduleInstanceAddr, providers, getResultValue)
		ret[addr] = rsrc
	}
	return ret
}

func compileModuleInstanceResource(
	ctx context.Context,
	config *configs.Resource,
	declScope exprs.Scope,
	moduleProviders configgraph.CompileProviderConfigRef,
	moduleInstanceAddr addrs.ModuleInstance,
	providers evalglue.ProvidersSchema,
	getResultValue func(context.Context, *configgraph.ResourceInstance, cty.Value, configgraph.Maybe[*configgraph.ProviderInstance], addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics),
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
		configEvalable = exprs.ForcedErrorEvalable(diags, tfdiags.SourceRangeFromHCL(config.TypeRange))
	} else if resourceTypeSchema == nil {
		suggestion := "TODO suggestion" //TODO NodeValidatableResource.noResourceSchemaSuggestion
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid resource type",
			Detail:   fmt.Sprintf("The provider %s does not support resource type %q.%s", config.Provider.ForDisplay(), resourceAddr.Type, suggestion),
			Subject:  config.TypeRange.Ptr(),
		})
		configEvalable = exprs.ForcedErrorEvalable(diags, tfdiags.SourceRangeFromHCL(config.TypeRange))
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
		InstanceSelector: compileInstanceSelector(ctx, declScope, config.ForEach, config.Count, config.Enabled),

		// The [configgraph.Resource] implementation will call back to this
		// for each child instance it discovers through [InstanceSelector],
		// allowing us to finalize all of the details for a specific instance
		// of this resource.
		CompileResourceInstance: func(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) *configgraph.ResourceInstance {
			localScope := instanceLocalScope(declScope, repData)
			providerRef := compileProviderConfigRef(ctx, moduleProviders, config.ProviderConfigAddr(), config.ProviderConfigRef, localScope)

			// For now we require a literal boolean constant in
			// create_before_destroy to match how the old implementation treated
			// this, but this is designed to grow to support arbitrary
			// expressions in future once we're ready to implement this issue:
			//    https://github.com/opentofu/opentofu/issues/2523
			var cbdVal cty.Value
			if config.Managed != nil {
				if !config.Managed.CreateBeforeDestroySet {
					cbdVal = cty.NullVal(cty.Bool)
				} else if config.Managed.CreateBeforeDestroy {
					cbdVal = cty.True
				} else {
					cbdVal = cty.False
				}
			}
			var cbdValuer *configgraph.OnceValuer
			if cbdVal != cty.NilVal {
				// We don't currently track the source location of the
				// create_before_destroy argument in particular, but when
				// we make this support arbitrary expressions in future
				// the expression's source range will be used here instead.
				cbdValuer = configgraph.ValuerOnce(
					exprs.ConstantValuerWithSourceRange(cbdVal, tfdiags.SourceRangeFromHCL(config.DeclRange)),
				)
			}

			additionalMarks := cty.ValueMarks{}
			// This adds an implicit depends_on from marks in repetition data
			maps.Copy(additionalMarks, repData.CountIndex.Marks())
			maps.Copy(additionalMarks, repData.EachKey.Marks())
			maps.Copy(additionalMarks, repData.EachValue.Marks())

			// Some language features related to resource blocks cause extra
			// transformations of the configuration value, so we'll deal
			// with those by transforming what we get from just evaluating
			// the main config body.
			configValuer := configgraph.ValuerOnce(exprs.DerivedValuer(
				exprs.NewClosure(
					configEvalable, localScope,
				),
				func(v cty.Value, diags tfdiags.Diagnostics) (cty.Value, tfdiags.Diagnostics) {
					if len(additionalMarks) != 0 {
						return v.WithMarks(additionalMarks), diags
					}
					return v, diags
				},
			))

			inst := &configgraph.ResourceInstance{
				Addr:                      absAddr.Instance(key),
				Provider:                  config.Provider,
				ConfigValuer:              configValuer,
				ProviderInstanceValuer:    configgraph.ValuerOnce(providerRef),
				CreateBeforeDestroyValuer: cbdValuer,
			}
			// Again the [ResourceInstance] implementation will call back
			// through this object so we can help it interact with the
			// appropriate provider and collect the result of whatever
			// side-effects our caller is doing with this resource instance
			// in the current phase. (The planned new state during the plan
			// phase, for example.)
			inst.Glue = &resourceInstanceGlue{
				getResultValue: func(ctx context.Context, configVal cty.Value, providerInst configgraph.Maybe[*configgraph.ProviderInstance], riDeps addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics) {
					return getResultValue(ctx, inst, configVal, providerInst, riDeps)
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
	getResultValue func(context.Context, cty.Value, configgraph.Maybe[*configgraph.ProviderInstance], addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics)
}

// ResultValue implements [configgraph.ResourceInstanceGlue].
func (r *resourceInstanceGlue) ResultValue(ctx context.Context, configVal cty.Value, providerInst configgraph.Maybe[*configgraph.ProviderInstance], riDeps addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics) {
	return r.getResultValue(ctx, configVal, providerInst, riDeps)
}
