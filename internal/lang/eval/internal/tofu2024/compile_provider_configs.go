// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"iter"

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

func compileModuleInstanceProviderConfigs(
	ctx context.Context,
	configs map[string]*configs.Provider,
	allResources iter.Seq[*configs.Resource],
	declScope exprs.Scope,
	reqdProviders map[string]*configs.RequiredProvider,
	moduleInstanceAddr addrs.ModuleInstance,
	providers evalglue.ProvidersSchema,
	validateProviderConfig func(context.Context, addrs.Provider, cty.Value) tfdiags.Diagnostics,
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
				Module: moduleInstanceAddr,
				Config: addrs.ProviderConfigCorrect{
					Provider: providerAddr,
					Alias:    config.Alias,
				},
			},
			ProviderAddr:     providerAddr,
			InstanceSelector: compileInstanceSelector(ctx, declScope, config.ForEach, nil, nil),
			CompileProviderInstance: func(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) *configgraph.ProviderInstance {
				instanceScope := instanceLocalScope(declScope, repData)
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
					ConfigValuer: configgraph.ValuerOnce(
						exprs.NewClosure(configEvalable, instanceScope),
					),
					ValidateConfig: func(ctx context.Context, v cty.Value) tfdiags.Diagnostics {
						return validateProviderConfig(ctx, providerAddr, v)
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
					Module: moduleInstanceAddr,
					Config: addrs.ProviderConfigCorrect{
						Provider: providerAddr,
					},
				},
				ProviderAddr:     providerAddr,
				InstanceSelector: compileInstanceSelector(ctx, declScope, nil, nil, nil),
				CompileProviderInstance: func(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) *configgraph.ProviderInstance {
					instanceScope := instanceLocalScope(declScope, repData)
					return &configgraph.ProviderInstance{
						Addr: addrs.AbsProviderInstanceCorrect{
							Config: addrs.AbsProviderConfigCorrect{
								Module: addrs.RootModuleInstance,
								Config: addrs.ProviderConfigCorrect{
									Provider: providerAddr,
								},
							},
							Key: key,
						},
						ProviderAddr: providerAddr,
						ConfigValuer: configgraph.ValuerOnce(
							exprs.NewClosure(configEvalable, instanceScope),
						),
						ValidateConfig: func(ctx context.Context, v cty.Value) tfdiags.Diagnostics {
							return validateProviderConfig(ctx, providerAddr, v)
						},
					}
				},
			}
		}
	}

	return ret
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
