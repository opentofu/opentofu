// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"

	otelAttr "go.opentelemetry.io/otel/attribute"
	otelTrace "go.opentelemetry.io/otel/trace"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tracing"
	"github.com/zclconf/go-cty/cty"
)

// NodePlanDestroyableResourceInstance represents a resource that is ready
// to be planned for destruction.
type NodePlanDestroyableResourceInstance struct {
	*NodeAbstractResourceInstance

	// skipRefresh indicates that we should skip refreshing
	skipRefresh bool
}

var (
	_ GraphNodeModuleInstance       = (*NodePlanDestroyableResourceInstance)(nil)
	_ GraphNodeReferenceable        = (*NodePlanDestroyableResourceInstance)(nil)
	_ GraphNodeReferencer           = (*NodePlanDestroyableResourceInstance)(nil)
	_ GraphNodeDestroyer            = (*NodePlanDestroyableResourceInstance)(nil)
	_ GraphNodeConfigResource       = (*NodePlanDestroyableResourceInstance)(nil)
	_ GraphNodeResourceInstance     = (*NodePlanDestroyableResourceInstance)(nil)
	_ GraphNodeAttachResourceConfig = (*NodePlanDestroyableResourceInstance)(nil)
	_ GraphNodeAttachResourceState  = (*NodePlanDestroyableResourceInstance)(nil)
	_ GraphNodeExecutable           = (*NodePlanDestroyableResourceInstance)(nil)
	_ GraphNodeProviderConsumer     = (*NodePlanDestroyableResourceInstance)(nil)
)

// GraphNodeDestroyer
func (n *NodePlanDestroyableResourceInstance) DestroyAddr() *addrs.AbsResourceInstance {
	addr := n.ResourceInstanceAddr()
	return &addr
}

// GraphNodeEvalable
func (n *NodePlanDestroyableResourceInstance) Execute(ctx context.Context, evalCtx EvalContext, op walkOperation) (diags tfdiags.Diagnostics) {
	addr := n.ResourceInstanceAddr()

	ctx, span := tracing.Tracer().Start(
		ctx, traceNamePlanResourceInstance,
		otelTrace.WithAttributes(
			otelAttr.String(traceAttrResourceInstanceAddr, addr.String()),
			otelAttr.Bool(traceAttrPlanRefresh, !n.skipRefresh),
		),
	)
	defer span.End()

	diags = diags.Append(n.resolveProvider(evalCtx, false, states.NotDeposed))
	if diags.HasErrors() {
		tracing.SetSpanError(span, diags)
		return diags
	}
	span.SetAttributes(
		otelAttr.String(traceAttrProviderInstanceAddr, traceProviderInstanceAddr(n.ResolvedProvider.ProviderConfig, n.ResolvedProviderKey)),
	)

	switch addr.Resource.Resource.Mode {
	case addrs.ManagedResourceMode:
		diags = diags.Append(
			n.managedResourceExecute(ctx, evalCtx, op),
		)
	case addrs.DataResourceMode:
		diags = diags.Append(
			n.dataResourceExecute(ctx, evalCtx, op),
		)
	default:
		panic(fmt.Errorf("unsupported resource mode %s", n.Config.Mode))
	}
	tracing.SetSpanError(span, diags)
	return diags
}

func (n *NodePlanDestroyableResourceInstance) managedResourceExecute(ctx context.Context, evalCtx EvalContext, _ walkOperation) (diags tfdiags.Diagnostics) {
	addr := n.ResourceInstanceAddr()

	// Declare a bunch of variables that are used for state during
	// evaluation. These are written to by address in the EvalNodes we
	// declare below.
	var change *plans.ResourceInstanceChange
	var state *states.ResourceInstanceObject

	state, err := n.readResourceInstanceState(ctx, evalCtx, addr)
	diags = diags.Append(err)
	if diags.HasErrors() {
		return diags
	}

	// If we are in the "skip refresh" mode then we will have skipped over our
	// usual opportunity to update the previous run state and refresh state
	// with the result of any provider schema upgrades, so we'll compensate
	// by doing that here.
	//
	// NOTE: this is coupled with logic in Context.destroyPlan which skips
	// running a normal plan walk when refresh is enabled. These two
	// conditionals must agree (be exactly opposite) in order to get the
	// correct behavior in both cases.
	if n.skipRefresh {
		diags = diags.Append(n.writeResourceInstanceState(ctx, evalCtx, state, prevRunState))
		if diags.HasErrors() {
			return diags
		}
		diags = diags.Append(n.writeResourceInstanceState(ctx, evalCtx, state, refreshState))
		if diags.HasErrors() {
			return diags
		}
	}

	change, destroyPlanDiags := n.planDestroy(ctx, evalCtx, state, "")
	diags = diags.Append(destroyPlanDiags)
	if diags.HasErrors() {
		return diags
	}

	diags = diags.Append(n.writeChange(ctx, evalCtx, change, ""))
	if diags.HasErrors() {
		return diags
	}

	diags = diags.Append(n.checkPreventDestroy(change))
	return diags
}

func (n *NodePlanDestroyableResourceInstance) dataResourceExecute(ctx context.Context, evalCtx EvalContext, _ walkOperation) (diags tfdiags.Diagnostics) {

	// We may not be able to read a prior data source from the state if the
	// schema was upgraded and we are destroying before ever refreshing that
	// data source. Regardless, a data source  "destroy" is simply writing a
	// null state, which we can do with a null prior state too.
	change := &plans.ResourceInstanceChange{
		Addr:        n.ResourceInstanceAddr(),
		PrevRunAddr: n.prevRunAddr(evalCtx),
		Change: plans.Change{
			Action: plans.Delete,
			Before: cty.NullVal(cty.DynamicPseudoType),
			After:  cty.NullVal(cty.DynamicPseudoType),
		},
		ProviderAddr: n.ResolvedProvider.ProviderConfig,
	}
	return diags.Append(n.writeChange(ctx, evalCtx, change, ""))
}
