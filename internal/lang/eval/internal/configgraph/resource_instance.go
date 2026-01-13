// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"fmt"
	"iter"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type ResourceInstance struct {
	// Addr is the absolute address of this resource instance, which is used
	// to track the resource instance between plan/apply rounds and between
	// the plan and apply phases in a single round.
	//
	// Placeholder addresses (where the IsPlaceholder method returns true) are
	// allowed here, representing that the containing object is actually
	// itself a placeholder for zero or more resource instances whose existence
	// and addresses we cannot determine yet.
	Addr addrs.AbsResourceInstance

	// Provider is the provider that this resource's type belongs to. This
	// is the provider to use when asking for config validation, etc.
	Provider addrs.Provider

	// ConfigValuer is a valuer for producing the object value representing
	// the configuration for this object. How the final configuration value
	// is chosen is decided by whatever created this object, but most typically
	// it's by the instance-compilation logic in the parent [Resource].
	ConfigValuer *OnceValuer

	// ProviderInstanceValuer is a valuer for producing a value representing
	// the provider instance that this resource instance is associated with.
	//
	// This valuer should return a value of the capsule type produced by passing
	// the address from the Provider field into [ProviderInstanceRefType],
	// or else type mismatch errors will be reported during evaluation.
	ProviderInstanceValuer *OnceValuer

	// Glue is provided by the system that "compiled" this [ResourceInstance]
	// object to allow calling back into that system to ask further questions
	// that arise dynamically during evaluation but whose results vary based
	// on concerns that our outside this package's scope.
	Glue ResourceInstanceGlue

	// valueOnce helps us to memoize the result from [ResourceInstance.Value]
	// so that we'll definitely return a consistent value to every call without
	// re-running whatever logic is behind the [ResourceInstance.Glue]
	// implementation, which might involve side-effects that could produce
	// different results
	valueOnce grapheval.Once[cty.Value]
}

var _ exprs.Valuer = (*ResourceInstance)(nil)

// IsExpansionPlaceholder returns true if this object is acting as a placeholder
// for zero or more instances whose existence and addresses cannot be decided
// yet, because the expansion rule depends on information that isn't known yet.
func (ri *ResourceInstance) IsExpansionPlaceholder() bool {
	return ri.Addr.IsPlaceholder()
}

// StaticCheckTraversal implements exprs.Valuer.
func (ri *ResourceInstance) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	return ri.ConfigValuer.StaticCheckTraversal(traversal)
}

// Value implements exprs.Valuer.
func (ri *ResourceInstance) Value(ctx context.Context) (v cty.Value, diags tfdiags.Diagnostics) {
	return ri.valueOnce.Do(ctx, func(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
		// TODO: Preconditions? Or should that be handled in the parent [Resource]
		// before we even attempt instance expansion? (Need to check the current
		// behavior in the existing system, to see whether preconditions guard
		// instance expansion.)
		// If we take preconditions into account here then we must transfer
		// [ResourceInstanceMark] marks from the check rule expressions into
		// configVal because config evaluation indirectly depends on those
		// references.

		// We use the configuration value here only for its marks, since that
		// allows us to propagate any
		configVal, diags := ri.ConfigValuer.Value(ctx)
		if diags.HasErrors() {
			// If we don't have a valid config value then we'll stop early
			// with an unknown value placeholder so that the external process
			// responsible for providing the result value can assume that it
			// will only ever recieve validated configuration values.
			return exprs.AsEvalError(cty.DynamicVal), diags
		}

		providerInst, providerInstMarks, moreDiags := ri.ProviderInstance(ctx)
		diags = diags.Append(moreDiags)
		if diags.HasErrors() {
			return exprs.AsEvalError(cty.DynamicVal), diags
		}

		riDeps := addrs.MakeSet[addrs.AbsResourceInstance]()
		for depInst := range ContributingResourceInstances(configVal) {
			if depInst != ri {
				riDeps.Add(depInst.Addr)
			}
		}

		// We also need help from our caller to prepare the final value to
		// return here, because it should reflect the outcome of whatever
		// resource-instance-related side effects we're doing this evaluation in
		// support of. Refer to the documentation of the ResultValue method
		// for details on what we're expecting this to do.
		resultVal, diags := ri.Glue.ResultValue(ctx, configVal, providerInst, riDeps)

		// We must pass the marks from the provider instance selection into the
		// result because the values that were returned may vary depending on
		// the provider configuration.
		resultVal = resultVal.WithMarks(providerInstMarks)

		// TODO: Postconditions, and transfer [ResourceInstanceMark] marks from
		// the check rule expressions onto resultVal because the presence of
		// a valid result value indirectly depends on those references.

		// The result needs some additional preparation to make sure it's
		// marked correctly for ongoing use in other expressions.
		return exprs.EvalResult(prepareResourceInstanceResult(resultVal, ri, configVal), diags)

	})
}

func (ri *ResourceInstance) ProviderInstance(ctx context.Context) (Maybe[*ProviderInstance], cty.ValueMarks, tfdiags.Diagnostics) {
	v, diags := ri.ProviderInstanceValuer.Value(ctx)
	if diags.HasErrors() {
		return nil, cty.NewValueMarks(exprs.EvalError), diags
	}
	inst, marks, err := ProviderInstanceFromValue(v, ri.Provider)
	if err != nil {
		marks[exprs.EvalError] = struct{}{}
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider instance reference",
			Detail:   fmt.Sprintf("Unsuitable provider selection for %s: %s.", ri.Addr, tfdiags.FormatError(err)),
			Subject:  MaybeHCLSourceRange(ri.ProviderInstanceValuer.ValueSourceRange()),
		})
		return nil, marks, diags
	}
	return inst, marks, diags
}

// ResourceInstanceDependencies returns a sequence of any other resource
// instances whose results this resource instance depends on.
//
// The result of this is trustworthy only if [ResourceInstance.CheckAll]
// returns without diagnostics. If errors are present then the result is
// best-effort but likely to be incomplete.
func (ri *ResourceInstance) ResourceInstanceDependencies(ctx context.Context) iter.Seq[*ResourceInstance] {
	// FIXME: This should also take into account:
	// - indirect references through the configuration of the provider instance
	//   this resource instance uses (which should arrive as marks on the
	//   [ProviderInstanceRefType] value that represents the provider instance),
	//   once we've actually got a Valuer to return the provider instance
	//   reference value.
	// - explicit dependencies in the depends_on argument
	// - ....anything else?
	//
	// We should NOT need to take into account dependencies of the parent
	// resource's InstanceSelector because substitutions of
	// count.index/each.key/each.value will transfer those in automatically by
	// the RepetitionData values being marked.

	// We ignore diagnostics here because callers should always perform a
	// CheckAll tree walk, including a visit to this resource instance object,
	// before trusting anything else that any configgraph nodes report.
	resultVal := diagsHandledElsewhere(ri.Value(ctx))

	// Our Value method always marks its result as depending on this
	// resource instance so that any expressions that refer to it will
	// be treated as depending on this resource instance, but we want
	// to filter that out here because otherwise we'd be reporting that
	// this resource depends on itself, which is impossible and confusing.
	return func(yield func(*ResourceInstance) bool) {
		for depInst := range ContributingResourceInstances(resultVal) {
			if depInst != ri {
				if !yield(depInst) {
					return
				}
			}
		}
	}
}

// ValueSourceRange implements exprs.Valuer.
func (ri *ResourceInstance) ValueSourceRange() *tfdiags.SourceRange {
	return ri.ConfigValuer.ValueSourceRange()
}

// CheckAll implements allChecker.
func (ri *ResourceInstance) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	var cg CheckGroup
	cg.CheckValuer(ctx, ri)
	return cg.Complete(ctx)
}

func (ri *ResourceInstance) AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo)) {
	announce(ri.ConfigValuer.RequestID(), grapheval.RequestInfo{
		Name:        fmt.Sprintf("configuration for %s", ri.Addr),
		SourceRange: ri.ConfigValuer.ValueSourceRange(),
	})
	announce(ri.valueOnce.RequestID(), grapheval.RequestInfo{
		Name:        fmt.Sprintf("final value for %s", ri.Addr),
		SourceRange: ri.ConfigValuer.ValueSourceRange(),
	})
	announce(ri.ProviderInstanceValuer.RequestID(), grapheval.RequestInfo{
		Name:        fmt.Sprintf("provider instance selection for %s", ri.Addr),
		SourceRange: ri.ProviderInstanceValuer.ValueSourceRange(),
	})
}
