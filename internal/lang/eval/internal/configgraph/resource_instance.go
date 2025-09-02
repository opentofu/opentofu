// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"fmt"

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

	// ConfigValuer is a valuer for producing the object value representing
	// the configuration for this object. How the final configuration value
	// is chosen is decided by whatever created this object, but most typically
	// it's by the instance-compilation logic in the parent [Resource].
	ConfigValuer *OnceValuer

	// GetResultValue is callback glue provided from outside this package
	// to integrate with any resource instance side-effects that are
	// being orchestrated elsewhere, such as getting the "planned new state"
	// of the resource instance during the plan phase, while keeping this
	// package focused only on the general concern of evaluating expressions
	// in the configuration.
	//
	// If this returns error diagnostics then it MUST also return a suitable
	// placeholder unknown value to use when evaluating downstream expressions.
	// If there's not enough information to return anything more precise
	// then returning [cty.DynamicVal] is an acceptable last resort.
	//
	// Real implementations of this are likely to block until some side-effects
	// have occured elsewhere, such as asking a provider to produce a planned
	// new state. If that external work depends on information coming from
	// any other part of this package's API then the implementation of that
	// MUST use the mechanisms from package grapheval in order to cooperate
	// with the self-dependency detection used by this package to prevent
	// deadlocks.
	GetResultValue func(ctx context.Context, configVal cty.Value) (cty.Value, tfdiags.Diagnostics)
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
func (ri *ResourceInstance) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	// NOTE WELL: All values returned from this function MUST be marked with
	// ResourceInstanceMark{ri} to achieve correct resource instance dependency
	// tracking.

	configVal, diags := ri.ConfigValuer.Value(ctx)
	if diags.HasErrors() {
		// We don't return the placeholder configVal here because a correct
		// final value for a resource instance needs to have had state
		// or provider-planned values injected into it and so configVal
		// alone (where unset arguments are always null) is not a correct
		// placeholder.
		return cty.DynamicVal.Mark(ResourceInstanceMark{ri}), diags
	}

	// We need some help from outside this package to prepare the final
	// value to return here, because it should reflect the outcome of
	// whatever resource-instance-related side effects we're doing
	// this evaluation in support of. Refer to the documentation of
	// the GetResultValue field for details on what we're expecting this
	// function to do.
	resultVal, moreDiags := ri.GetResultValue(ctx, configVal)
	diags = diags.Append(moreDiags)
	return resultVal.Mark(ResourceInstanceMark{ri}), diags
}

// ValueSourceRange implements exprs.Valuer.
func (ri *ResourceInstance) ValueSourceRange() *tfdiags.SourceRange {
	return ri.ConfigValuer.ValueSourceRange()
}

// CheckAll implements allChecker.
func (ri *ResourceInstance) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	var cg checkGroup
	cg.CheckValuer(ctx, ri)
	return cg.Complete(ctx)
}

func (ri *ResourceInstance) AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo)) {
	announce(ri.ConfigValuer.RequestID(), grapheval.RequestInfo{
		Name:        fmt.Sprintf("configuration for %s", ri.Addr),
		SourceRange: ri.ConfigValuer.ValueSourceRange(),
	})
}
