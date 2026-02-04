// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
	"github.com/opentofu/opentofu/internal/lang/eval"
)

////////////////////////////////////////////////////////////////////////////////
// This file contains methods of [execGraphBuilder] that are related to the
// parts of an execution graph that deal with resource instances of mode
// [addrs.ManagedResourceMode] in particular.
////////////////////////////////////////////////////////////////////////////////

// ManagedResourceSubgraph adds graph nodes needed to apply changes for a
// managed resource instance, and returns what should be used as its final
// result to propagate into to downstream references.
//
// TODO: This is definitely not sufficient for the full complexity of all of the
// different ways managed resources can potentially need to be handled in an
// execution graph. It's just a simple placeholder adapted from code that was
// originally written inline in [planGlue.planDesiredManagedResourceInstance]
// just to preserve the existing functionality for now until we design a more
// complete approach in later work.
func (b *execGraphBuilder) ManagedResourceInstanceSubgraph(desired *eval.DesiredResourceInstance, plannedValue cty.Value) execgraph.ResourceInstanceResultRef {
	// We need to explicitly model our dependency on any upstream resource
	// instances in the resource instance graph. These don't naturally emerge
	// from the data flow because these results are intermediated through the
	// evaluator, which indirectly incorporates the results into the
	// desiredInstRef result we'll build below.
	dependencyWaiter := b.waiterForResourceInstances(desired.RequiredResourceInstances.All())

	providerClientRef, closeProviderAfter := b.ProviderInstance(*desired.ProviderInstance, b.lower.Waiter())

	// FIXME: If this is one of the "replace" actions then we need to generate
	// a more complex graph that has two pairs of "final plan" and "apply".
	instAddrRef := b.lower.ConstantResourceInstAddr(desired.Addr)
	priorStateRef := b.lower.ResourceInstancePrior(instAddrRef)
	plannedValRef := b.lower.ConstantValue(plannedValue)
	desiredInstRef := b.lower.ResourceInstanceDesired(instAddrRef, dependencyWaiter)
	finalPlanRef := b.lower.ManagedFinalPlan(
		desiredInstRef,
		priorStateRef,
		plannedValRef,
		providerClientRef,
	)
	finalResultRef := b.lower.ManagedApply(
		finalPlanRef,
		execgraph.NilResultRef[*exec.ResourceInstanceObject](),
		providerClientRef,
	)
	closeProviderAfter(finalResultRef)

	return finalResultRef
}
