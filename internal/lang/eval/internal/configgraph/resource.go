// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Resource struct {
	// Addr is the absolute address of this resource, used as the basis of
	// the addresses used to track its instances between plan/apply rounds
	// and between the plan and apply phases in a single round.
	//
	// Placeholder addresses (where the IsPlaceholder method returns true) are
	// allowed here, representing that the containing object is actually
	// itself a placeholder for zero or more resources whose existence
	// and addresses we cannot determine yet.
	Addr addrs.AbsResource

	// Each instance of a resource gets evaluated in its own local scope
	// with extra symbols each.key, etc decided on a per-instance basis,
	// so initially we just track the unevaluated config and the parent
	// scope we'll eventually use when we are ready to lazily "compile"
	// each child [ResourceInstance].
	ConfigEvalable exprs.Evalable
	ParentScope    exprs.Scope

	// InstanceSelector represents a rule for deciding which instances of
	// this resource have been declared.
	InstanceSelector InstanceSelector

	DeclRange tfdiags.SourceRange

	// instancesResult tracks the process of deciding which instances are
	// currently declared for this resource, and the result of that process.
	//
	// Only the decideInstances method accesses this directly. Use that
	// method to obtain the coalesced result for use elsewhere.
	instancesResult grapheval.Once[*compiledInstances[*ResourceInstance]]
}

var _ exprs.Valuer = (*Resource)(nil)

// IsExpansionPlaceholder returns true if this object is acting as a placeholder
// for zero or more resources whose existence and addresses cannot be decided
// yet, because the expansion rule depends on information that isn't known yet.
//
// Note that at the Resource level this means that one of the modules this
// resource is nested within has an unknown set of instances, rather than
// that this resource's own expansion is not known. Unknown expansion of the
// resource itself is represented by producing a single [ResourceInstance]
// which is a placeholder itself, as reported by
// [ResourceInstance.IsExpansionPlaceholder].
func (r *Resource) IsExpansionPlaceholder() bool {
	return r.Addr.IsPlaceholder()
}

// Instances returns the instances that are selected for this resource in its
// configuration, without evaluating their configuration objects yet.
//
// Use this when performing a tree walk to discover resource instances to
// make sure that it's possible to tell whatever process is running alongside
// that it needs to produce a result value for a particular resource instance
// before we actually request that value.
func (r *Resource) Instances(ctx context.Context) map[addrs.InstanceKey]*ResourceInstance {
	// We ignore the diagnostics here because they will be returned by
	// the Value method instead.
	result, _ := r.decideInstances(ctx)
	return result.Instances
}

// StaticCheckTraversal implements exprs.Valuer.
func (r *Resource) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	return staticCheckTraversalForInstances(r.InstanceSelector, traversal)
}

// Value implements exprs.Valuer.
func (r *Resource) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	selection, diags := r.decideInstances(ctx)
	return valueForInstances(ctx, selection), diags
}

// ValueSourceRange implements exprs.Valuer.
func (r *Resource) ValueSourceRange() *tfdiags.SourceRange {
	return &r.DeclRange
}

func (r *Resource) decideInstances(ctx context.Context) (*compiledInstances[*ResourceInstance], tfdiags.Diagnostics) {
	return r.instancesResult.Do(ctx, func(ctx context.Context) (*compiledInstances[*ResourceInstance], tfdiags.Diagnostics) {
		return compileInstances(ctx, r.InstanceSelector, r.compileInstance)
	})
}

func (r *Resource) compileInstance(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) *ResourceInstance {
	ret := &ResourceInstance{
		Addr: r.Addr.Instance(key),
		ConfigValuer: ValuerOnce(exprs.NewClosure(
			r.ConfigEvalable,
			instanceLocalScope(r.ParentScope, repData),
		)),
	}
	return ret
}
