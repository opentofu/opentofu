// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/hcl2shim"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// The symbols in this file deal with the weird special cases that the current
// OpenTofu language has for propagating provider configs between modules
// and selecting a provider instance for each resource instance.
//
// Hopefully eventually we'll move away from this to just treating provider
// instance references as a normal kind of value that can pass through input
// variables and output values and appear as part of larger data structures,
// but for now this logic is acting as a translation layer so that [configs]
// can continue to treat providers in this weird special way while [configgraph]
// _thinks_ they are just normal values of a special type.

// rootMissingProviders is used to ferry the dynamically injected missing providers
// into the root module's ProviderConfig lookup
type rootMissingProviders struct {
	lock            sync.Mutex
	providerConfigs map[addrs.LocalProviderConfig]*configgraph.ProviderConfig
}

func (r *rootMissingProviders) getOk(localAddr addrs.LocalProviderConfig) (*configgraph.ProviderConfig, bool) {
	r.lock.Lock()
	defer r.lock.Unlock()
	ret, ok := r.providerConfigs[localAddr]
	return ret, ok
}

// compileProviderConfigRefMissingInRoot builds the "base" provider ref compiler used by the root module
// This specifically handles the legacy edge cases where a resource references a provider that does not
// have a corresponding provider configuration. This path injects an implicit "fake/empty" provider config
// in the root module.
//
// This matches some complex interdependencies of the existing [tofu.MissingProviderTransformer], combined
// with a nice ball of spaghetti logic throughout the tofu package.
//
// We return both the compiler function and the map of references that will be updated dynamically. The
// map of references should be used in the root module for ProviderConfig lookups.
func compileProviderConfigRefMissingInRoot(
	requiredProviders map[string]*configs.RequiredProvider,
	providers evalglue.ProvidersSchema,
	validateProviderConfig func(context.Context, addrs.Provider, cty.Value) tfdiags.Diagnostics,
	extraMarks cty.ValueMarks,
) (configgraph.CompileProviderConfigRef, *rootMissingProviders) {
	missing := &rootMissingProviders{
		providerConfigs: map[addrs.LocalProviderConfig]*configgraph.ProviderConfig{},
	}

	return func(ctx context.Context, providerInstAddr addrs.LocalProviderConfig) exprs.Valuer {
		missing.lock.Lock()
		defer missing.lock.Unlock()

		// Check to see if we have a corresponding required_providers entry with an alias. This is
		// explicitly to support validating a non-root module.
		//
		// TODO: only enable this during the validation pass and forbid for other operations
		for _, required := range requiredProviders {
			for _, alias := range required.Aliases {
				if alias == providerInstAddr {
					providerInstAddr.Alias = ""
					break
				}
			}
		}

		// Aliases are not supported in the provider fallback case
		if providerInstAddr.Alias != "" {
			diags := tfdiags.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Reference to undeclared provider configuration",
				Detail:   fmt.Sprintf("There is no provider configuration %s declared in this module.", providerInstAddr.StringCompact()),
			})
			return exprs.ForcedErrorValuer(diags)
		}

		// Have we already seen this one before?
		if missingConfig, ok := missing.providerConfigs[providerInstAddr]; ok {
			return &sidechannelProviderInstanceRefValuer{
				localAddr:  providerInstAddr,
				mainValuer: missingConfig,
			}
		}

		// Assume this provider is within the hashicorp namespace and construct an empty provider config
		// TODO ensure this chain works with google vs google-beta
		emptyConfig := &configs.Provider{
			Name:   providerInstAddr.LocalName,
			Config: hcl2shim.SynthBody(providerInstAddr.String(), make(map[string]cty.Value)),
		}
		missingConfig := compileProviderConfig(ctx, emptyConfig, nil, requiredProviders, addrs.RootModuleInstance, providers, validateProviderConfig, extraMarks)
		missing.providerConfigs[providerInstAddr] = missingConfig

		return &sidechannelProviderInstanceRefValuer{
			localAddr:  providerInstAddr,
			mainValuer: missingConfig,
		}
	}, missing
}

// compileProviderConfigRefModule adds the provider configs declared in the module to the ref lookup chain
func compileProviderConfigRefModule(
	parentCompiler configgraph.CompileProviderConfigRef,
	local map[addrs.LocalProviderConfig]*configgraph.ProviderConfig,
) configgraph.CompileProviderConfigRef {
	return func(ctx context.Context, providerInstAddr addrs.LocalProviderConfig) exprs.Valuer {

		if localConfig, ok := local[providerInstAddr]; ok {
			return &sidechannelProviderInstanceRefValuer{
				localAddr:   providerInstAddr,
				mainValuer:  localConfig,
				sourceRange: localConfig.DeclRange.ToHCL().Ptr(),
			}
		}

		// Not a local config, let's look back up the tree
		return parentCompiler(ctx, providerInstAddr)
	}
}

// compileProviderConfigRefProxy applies the `providers = { ... }` logic to the ref lookup chain.
// TODO consider if this should have additional validation using the required_providers block. It's currently handled in the configs package directly.
func compileProviderConfigRefProxy(
	parentCompiler configgraph.CompileProviderConfigRef,
	passed []configs.PassedProviderConfig,
	evalScope exprs.Scope,
) configgraph.CompileProviderConfigRef {
	if passed == nil {
		// Legacy (implicit)
		return parentCompiler
	}

	return func(ctx context.Context, providerInstAddr addrs.LocalProviderConfig) exprs.Valuer {

		// modern (explicit)
		for _, p := range passed {
			// This works because InChild can't have a key expression
			if p.InChild.Name == providerInstAddr.LocalName && p.InChild.Alias == providerInstAddr.Alias {
				parentLocalAddr := addrs.LocalProviderConfig{
					LocalName: p.InParent.Name,
					Alias:     p.InParent.Alias,
				}

				return newProviderInstanceRefValuer(parentCompiler(ctx, parentLocalAddr), providerInstAddr, p.InParent, evalScope)
			}
		}
		// Fallback to legacy, this seems to match the current behavior?
		return parentCompiler(ctx, providerInstAddr)

		// TODO: Make this error message better by talking about what's missing
		// in config in terms more familiar to a module author, including the
		// various ways provider instances can be implied or inherited, and try
		// using "didyoumean" to see if we have something similar they might
		// have been trying to refer to.
		/*diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Reference to undeclared provider configuration",
			Detail:   fmt.Sprintf("There is no provider configuration %s declared in this module.", providerInstAddr.StringCompact()),
			Subject:  wholeRange,
		})
		return exprs.ForcedErrorValuer(diags)*/
	}
}

// compileProviderConfigRef uses a resource's provider config ref to dereference the
func compileProviderConfigRef(
	ctx context.Context,
	parentCompiler configgraph.CompileProviderConfigRef,
	providerInstAddr addrs.LocalProviderConfig,
	ref *configs.ProviderConfigRef,
	evalScope exprs.Scope,
) exprs.Valuer {
	return newProviderInstanceRefValuer(
		parentCompiler(ctx, providerInstAddr),
		providerInstAddr,
		ref,
		evalScope,
	)
}

// Helper function to turn a [configs.ProviderConfigRef] into a [sidechannelProviderInstanceRefValuer]
func newProviderInstanceRefValuer(
	mainValuer exprs.Valuer,
	localAddr addrs.LocalProviderConfig,
	ref *configs.ProviderConfigRef,
	keyScope exprs.Scope,
) exprs.Valuer {
	var wholeRange *hcl.Range
	if ref != nil {
		wholeRange = &ref.NameRange
		if ref.AliasRange != nil {
			wholeRange = hcl.RangeBetween(ref.NameRange, *ref.AliasRange).Ptr()
		}
	}

	// If we have a key expression then we'll compile it into a closure to
	// be resolved in our "key" scope.
	var instanceKeyValuer exprs.Valuer
	if ref != nil && ref.KeyExpression != nil {
		instanceKeyValuer = exprs.NewClosure(
			exprs.EvalableHCLExpression(ref.KeyExpression),
			keyScope,
		)
	}

	return &sidechannelProviderInstanceRefValuer{
		mainValuer:        mainValuer,
		instanceKeyValuer: instanceKeyValuer,
		sourceRange:       wholeRange,
		localAddr:         localAddr,
	}
}

type sidechannelProviderInstanceRefValuer struct {
	mainValuer        exprs.Valuer
	instanceKeyValuer exprs.Valuer // nil when there's no instance key expression
	sourceRange       *hcl.Range
	localAddr         addrs.LocalProviderConfig
}

// StaticCheckTraversal implements exprs.Valuer.
func (s *sidechannelProviderInstanceRefValuer) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	// we'll just deal with everything dynamically here, because there
	// aren't any weird facilities like the "try" function to worry about
	// in the provider side-channel and so static vs. dynamic doesn't
	// make any significant difference.
	return nil
}

// Value implements exprs.Valuer.
func (s *sidechannelProviderInstanceRefValuer) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	return s.value(ctx, true)
}

// value allows us to only check the "last" provider in the lookup chain for missing key value expressions.
func (s *sidechannelProviderInstanceRefValuer) value(ctx context.Context, directProviderReferenceRequired bool) (cty.Value, tfdiags.Diagnostics) {
	var firstVal cty.Value
	var diags tfdiags.Diagnostics
	if sideChannelProvider, ok := s.mainValuer.(*sidechannelProviderInstanceRefValuer); ok {
		firstVal, diags = sideChannelProvider.value(ctx, false)
	} else {
		firstVal, diags = s.mainValuer.Value(ctx)
	}
	if diags.HasErrors() {
		return exprs.AsEvalError(cty.DynamicVal), diags
	}

	firstTy := firstVal.Type()
	if configgraph.IsProviderInstanceRefType(firstTy) {
		if s.instanceKeyValuer != nil {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid provider instance reference",
				Detail:   fmt.Sprintf("Provider configuration %s does not use for_each, so a reference to it must not include an instance key.", s.localAddr),
				Subject:  s.sourceRange,
			})
			return exprs.AsEvalError(cty.DynamicVal), diags
		}
		return firstVal, diags
	} else if firstTy.IsObjectType() {
		if s.instanceKeyValuer == nil {
			if directProviderReferenceRequired {
				// Configuration error, user did not specify a key expression where required
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid provider instance reference",
					Detail:   fmt.Sprintf("Provider configuration %s uses for_each, so a reference to it must specify which instance to select using syntax like %s[\"example\"].", s.localAddr, s.localAddr),
					Subject:  s.sourceRange,
				})
				return exprs.AsEvalError(cty.DynamicVal), diags
			}
			return firstVal, diags
		}
		keyVal, moreDiags := s.instanceKeyValuer.Value(ctx)
		diags = diags.Append(moreDiags)
		if moreDiags.HasErrors() {
			return exprs.AsEvalError(cty.DynamicVal), diags
		}
		// We'll use HCL's "index" implementation here so that it'll treat
		// things the same way as HCL would normally treat its index operator
		// in a dynamic expression context, and return the same errors we're
		// familiar with.
		finalVal, hclDiags := hcl.Index(firstVal, keyVal, configgraph.MaybeHCLSourceRange(s.instanceKeyValuer.ValueSourceRange()))
		diags = diags.Append(hclDiags)
		if hclDiags.HasErrors() {
			return exprs.AsEvalError(cty.DynamicVal), diags
		}
		return finalVal, diags
	} else {
		// In this weird sidechannel land, firstVal should be either a single
		// provider instance reference or an object caused by using for_each
		// in the provider config block. Anything else represents a bug in
		// whatever built s.mainValuer.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider instance reference",
			Detail:   fmt.Sprintf("The provider reference expression %s produced %#v instead of a provider reference. This is a bug in OpenTofu.", s.localAddr, firstTy),
			Subject:  s.sourceRange,
		})
		return exprs.AsEvalError(cty.DynamicVal), diags

	}
}

// ValueSourceRange implements exprs.Valuer.
func (s *sidechannelProviderInstanceRefValuer) ValueSourceRange() *tfdiags.SourceRange {
	if s.sourceRange == nil {
		return nil
	}
	rng := tfdiags.SourceRangeFromHCL(*s.sourceRange)
	return &rng
}
