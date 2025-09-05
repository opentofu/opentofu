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
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
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

type moduleProvidersSideChannel struct {
	// instanceVals are [exprs.Valuers] that produce values that are either
	// individual provider instance references or objects whose attribute
	// values are provider instance references. This is where we look to
	// handle the weirdo sidechannel provider reference syntax, like
	// "aws.foo" to refer to a configuration for whatever provider has the
	// local name "aws" that has the alias "foo".
	//
	// In practice this can contain a mixture of valuers that were passed
	// in from a parent module (using the "providers" argument in a module
	// block) and valuers that refer to provider configurations declared
	// within this module. We don't distinguish those two cases here
	// because both are mapped together into a single namespace per module.
	instanceVals map[addrs.LocalProviderConfig]exprs.Valuer
}

func compileModuleProvidersSidechannel(_ context.Context, fromParent map[addrs.LocalProviderConfig]exprs.Valuer, local map[addrs.LocalProviderConfig]*configgraph.ProviderConfig) *moduleProvidersSideChannel {
	instanceVals := make(map[addrs.LocalProviderConfig]exprs.Valuer, len(fromParent)+len(local))
	maps.Copy(instanceVals, fromParent)
	for addr, node := range local {
		instanceVals[addr] = node
	}

	return &moduleProvidersSideChannel{
		instanceVals: instanceVals,
	}
}

// CompileProviderConfigRef compiles the given provider config reference into
// an [exprs.Valuer] that returns a provider instance reference value.
//
// evalScope is the scope to be used if the reference includes a dynamic
// instance key. It should be a scope that includes instance-specific symbols
// like each.key for whatever object the provider config reference belongs to.
func (psc *moduleProvidersSideChannel) CompileProviderConfigRef(ctx context.Context, providerInstAddr addrs.LocalProviderConfig, ref *configs.ProviderConfigRef, evalScope exprs.Scope) exprs.Valuer {
	var wholeRange *hcl.Range
	if ref != nil {
		wholeRange = &ref.NameRange
		if ref.AliasRange != nil {
			wholeRange = hcl.RangeBetween(ref.NameRange, *ref.AliasRange).Ptr()
		}
	}

	mainValuer, ok := psc.instanceVals[providerInstAddr]
	if !ok {
		var diags tfdiags.Diagnostics
		// TODO: Make this error message better by talking about what's missing
		// in config in terms more familiar to a module author, including the
		// various ways provider instances can be implied or inherited, and try
		// using "didyoumean" to see if we have something similar they might
		// have been trying to refer to.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Reference to undeclared provider configuration",
			Detail:   fmt.Sprintf("There is no provider configuration %s declared in this module.", providerInstAddr.StringCompact()),
			Subject:  wholeRange,
		})
		return exprs.ForcedErrorValuer(diags)
	}

	// If we have a key expression then we'll compile it into a closure to
	// be resolved in our "normal" scope.
	var instanceKeyValuer exprs.Valuer
	if ref != nil && ref.KeyExpression != nil {
		instanceKeyValuer = exprs.NewClosure(
			exprs.EvalableHCLExpression(ref.KeyExpression),
			evalScope,
		)
	}

	return &sidechannelProviderInstanceRefValuer{
		mainValuer:        mainValuer,
		instanceKeyValuer: instanceKeyValuer,
		sourceRange:       wholeRange,
		localAddr:         providerInstAddr,
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
	firstVal, diags := s.mainValuer.Value(ctx)
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
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid provider instance reference",
				Detail:   fmt.Sprintf("Provider configuration %s uses for_each, so a reference to it must specify which instance to select using syntax like %s[\"example\"].", s.localAddr, s.localAddr),
				Subject:  s.sourceRange,
			})
			return exprs.AsEvalError(cty.DynamicVal), diags
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
