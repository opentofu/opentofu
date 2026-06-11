// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"maps"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func compileModuleInstanceProviderConfigs(
	ctx context.Context,
	configs map[string]*configs.Provider,
	declScope exprs.Scope,
	reqdProviders map[string]*configs.RequiredProvider,
	moduleInstanceAddr addrs.ModuleInstance,
	providers evalglue.Providers,
	extraMarks cty.ValueMarks,
) map[addrs.LocalProviderConfig]*configgraph.ProviderConfig {
	ret := make(map[addrs.LocalProviderConfig]*configgraph.ProviderConfig, len(configs))

	for _, config := range configs {
		localAddr := addrs.LocalProviderConfig{
			LocalName: config.Name,
			Alias:     config.Alias,
		}
		ret[localAddr] = compileProviderConfig(ctx, config, declScope, reqdProviders, moduleInstanceAddr, providers, extraMarks)
	}

	return ret
}

func compileProviderConfig(
	ctx context.Context,
	config *configs.Provider,
	declScope exprs.Scope,
	reqdProviders map[string]*configs.RequiredProvider,
	moduleInstanceAddr addrs.ModuleInstance,
	providers evalglue.Providers,
	extraMarks cty.ValueMarks,
) *configgraph.ProviderConfig {
	providerAddr := addrs.NewDefaultProvider(config.Name)
	if reqd, ok := reqdProviders[config.Name]; ok {
		providerAddr = reqd.Type
	}

	var configEvalable exprs.Evalable
	configSchema, diags := providers.ProviderConfigSchema(ctx, providerAddr)
	if diags.HasErrors() {
		configEvalable = exprs.ForcedErrorEvalable(diags, tfdiags.SourceRangeFromHCL(config.DeclRange))
	} else {
		spec := configSchema.Block.DecoderSpec()
		configEvalable = exprs.EvalableHCLBodyWithDynamicBlocks(config.Config, spec)
	}

	return &configgraph.ProviderConfig{
		Addr: addrs.AbsProviderConfigCorrect{
			Module: moduleInstanceAddr,
			Config: addrs.ProviderConfigCorrect{
				Provider: providerAddr,
				Alias:    config.Alias,
			},
		},
		ProviderAddr:     providerAddr,
		InstanceSelector: compileInstanceSelector(ctx, declScope, config.ForEach, nil, nil, extraMarks),
		CompileProviderInstance: func(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) *configgraph.ProviderInstance {
			instanceScope := instanceLocalScope(declScope, repData)

			inheritedMarks := cty.ValueMarks{}
			// This adds an implicit depends_on from marks in repetition data
			maps.Copy(inheritedMarks, repData.CountIndex.Marks())
			maps.Copy(inheritedMarks, repData.EachKey.Marks())
			maps.Copy(inheritedMarks, repData.EachValue.Marks())
			maps.Copy(inheritedMarks, extraMarks) // preserve the extra marks from our caller too

			// Some language features related to resource blocks cause extra
			// transformations of the configuration value, so we'll deal
			// with those by transforming what we get from just evaluating
			// the main config body.
			configValuer := configgraph.ValuerOnce(exprs.DerivedValuer(
				exprs.NewClosure(configEvalable, instanceScope),
				func(v cty.Value, diags tfdiags.Diagnostics) (cty.Value, tfdiags.Diagnostics) {
					if len(inheritedMarks) != 0 {
						return v.WithMarks(inheritedMarks), diags
					}
					return v, diags
				},
			))

			return &configgraph.ProviderInstance{
				Addr: addrs.AbsProviderInstanceCorrect{
					Config: addrs.AbsProviderConfigCorrect{
						Module: addrs.RootModuleInstance,
						Config: addrs.ProviderConfigCorrect{
							Provider: providerAddr,
							Alias:    config.Alias,
						},
					},
					Key: key,
				},
				ProviderAddr: providerAddr,
				ConfigValuer: configValuer,
				ValidateConfig: func(ctx context.Context, v cty.Value) tfdiags.Diagnostics {
					return providers.ValidateProviderConfig(ctx, providerAddr, v)
				},
			}
		},
	}
}
