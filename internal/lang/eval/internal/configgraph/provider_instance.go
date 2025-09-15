// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"iter"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// ProviderInstance represents the configuration for an instance of a provider.
//
// Note that this type's name is slightly misleading because it does not
// represent an already-running provider that requests can be sent to, but
// rather the configuration that should be sent to a running instance of
// this provider in order to prepare it for use. This package does not deal
// with "configured" providers directly at all, instead expecting its caller
// (e.g. an implementation or the plan or apply phase) to handle the provider
// instance lifecycle.
type ProviderInstance struct {
	// Addr is the absolute address of this specific provider instance.
	Addr addrs.AbsProviderInstanceCorrect

	// ProviderAddr is the address of the provider this is an instance of.
	ProviderAddr addrs.Provider

	// ConfigValuer produces the object value representing the configuration
	// for this provider instance.
	ConfigValuer *OnceValuer

	// ValidateConfig is a function provided by whatever compiled this object
	// which takes the result of ConfigValuer and potentially returns additional
	// diagnostics typically based on validation logic built in to the provider
	// itself.
	ValidateConfig func(context.Context, cty.Value) tfdiags.Diagnostics

	validatedConfig grapheval.Once[cty.Value]
}

var _ exprs.Valuer = (*ProviderInstance)(nil)

// StaticCheckTraversal implements exprs.Valuer.
func (p *ProviderInstance) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if len(traversal) != 0 {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid reference to provider instance",
			Detail:   "A provider instance reference does not have any attributes or elements.",
			Subject:  traversal.SourceRange().Ptr(),
		})
	}
	return diags
}

// Value implements exprs.Valuer.
func (p *ProviderInstance) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	// The value for a provider instance as used in expressions is just an
	// opaque reference to this object represented as a cty capsule type,
	// but we do still evaluate the configuration first because that ensures
	// that everything the configuration depends on has been resolved and
	// allows us to transfer a subset of the marks to the reference value to
	// represent that any values produced by resources belonging to this
	// provider might vary by the provider's configuration.
	//
	// We don't propagate diagnostics here because otherwise they would
	// appear redundantly for every reference to the provider instance.
	// Instead, [ProviderInstance.CheckAll] calls
	// [ProviderInstance.ConfigValue] to expose its diagnostics directly.
	configVal := diagsHandledElsewhere(p.ConfigValue(ctx))

	// We copy over only the EvalError and resource-instance-reference-related
	// marks because it would be too conservative to copy others. For example,
	// it doesn't make sense to say that an ephemeral value anywhere in the
	// provider configuration causes all resource instances belonging to this
	// provider instance to also be ephemeral values.
	ret := ProviderInstanceRefValue(p)
	if configVal.HasMarkDeep(exprs.EvalError) {
		ret = ret.Mark(exprs.EvalError)
	}
	for ri := range ContributingResourceInstances(configVal) {
		ret = ret.Mark(ResourceInstanceMark{ri})
	}
	return ret, nil
}

// ConfigValue returns an object representing the configuration that should
// be sent to a provider to make it behave as the configured provider instance.
//
// This value should not bt exposed for references from expressions elsewhere
// in the configuration. The result is considered private to the provider
// process that is configured with it.
func (p *ProviderInstance) ConfigValue(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	// We use a "Once" here to coalesce to just one ValidateConfig call per
	// ProviderInstance object, even when multiple callers ask for the
	// configuration for this instance.
	return p.validatedConfig.Do(ctx, func(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
		v, diags := p.ConfigValuer.Value(ctx)
		if diags.HasErrors() {
			return cty.DynamicVal, diags
		}
		moreDiags := p.ValidateConfig(ctx, v)
		diags = diags.Append(moreDiags)
		if diags.HasErrors() {
			return cty.DynamicVal, diags
		}
		return v, diags
	})
}

// ResourceInstanceDependencies returns a sequence of any resource instances
// whose results the configuration of this provider instance depends on.
//
// The result of this is trustworthy only if [ProviderInstance.CheckAll]
// returns without diagnostics. If errors are present then the result is
// best-effort but likely to be incomplete.
func (p *ProviderInstance) ResourceInstanceDependencies(ctx context.Context) iter.Seq[*ResourceInstance] {
	// FIXME: This should also take into account:
	// - explicit dependencies in the depends_on argument
	// - ....anything else?
	//
	// We should NOT need to take into account dependencies of the parent
	// provider config's InstanceSelector because substitutions of
	// count.index/each.key/each.value will transfer those in automatically by
	// the RepetitionData values being marked.

	// We ignore diagnostics here because callers should always perform a
	// CheckAll tree walk, including a visit to this provider instance object,
	// before trusting anything else that any configgraph nodes report.
	resultVal := diagsHandledElsewhere(p.ConfigValue(ctx))
	return ContributingResourceInstances(resultVal)
}

// ValueSourceRange implements exprs.Valuer.
func (p *ProviderInstance) ValueSourceRange() *tfdiags.SourceRange {
	// TODO: Does it make sense to return the source range of the provider
	// configuration block here, or is that confusing because our value
	// is a reference to a specific instance?
	return nil
}

// CheckAll implements allChecker.
func (p *ProviderInstance) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	var cg CheckGroup
	cg.CheckValuer(ctx, p)
	// We also need to directly check the ConfigValue method, because diags
	// from there do not propagate through Value.
	cg.CheckDiagsFunc(ctx, func(ctx context.Context) tfdiags.Diagnostics {
		_, diags := p.ConfigValue(ctx)
		return diags
	})
	return cg.Complete(ctx)
}

func (p *ProviderInstance) AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo)) {
	announce(p.ConfigValuer.RequestID(), grapheval.RequestInfo{
		Name:        p.Addr.String() + " configuration",
		SourceRange: p.ConfigValuer.ValueSourceRange(),
	})
}
