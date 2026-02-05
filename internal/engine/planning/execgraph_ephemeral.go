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
// [addrs.EphemeralResourceMode] in particular.
////////////////////////////////////////////////////////////////////////////////

// EphemeralResourceSubgraph adds graph nodes needed to apply changes for a
// ephemeral resource instance, and returns what should be used as its final
// result to propagate into to downstream references.
//
// TODO: This is definitely not sufficient for the full complexity of all of the
// different ways ephemeral resources can potentially need to be handled in an
// execution graph. It's just a simple placeholder adapted from code that was
// originally written inline in [planGlue.planDesiredEphemeralResourceInstance]
// just to preserve the existing functionality for now until we design a more
// complete approach in later work.
func (b *execGraphBuilder) EphemeralResourceInstanceSubgraph(desired *eval.DesiredResourceInstance, plannedValue cty.Value, providerClientRef execgraph.ResultRef[*exec.ProviderClient]) execgraph.ResourceInstanceResultRef {
	b.mu.Lock()
	defer b.mu.Unlock()

	closeWait, registerCloseBlocker := b.makeCloseBlocker()
	b.openEphemeralRefs.Put(desired.Addr, registerCloseBlocker)

	// We need to explicitly model our dependency on any upstream resource
	// instances in the resource instance graph. These don't naturally emerge
	// from the data flow because these results are intermediated through the
	// evaluator, which indirectly incorporates the results into the
	// desiredInstRef result we'll build below.
	dependencyWaiter, closeDependencyAfter := b.waiterForResourceInstances(desired.RequiredResourceInstances.All())

	instAddrRef := b.lower.ConstantResourceInstAddr(desired.Addr)
	desiredInstRef := b.lower.ResourceInstanceDesired(instAddrRef, dependencyWaiter)

	openRef := b.lower.EphemeralOpen(desiredInstRef, providerClientRef)
	stateRef := b.lower.EphemeralState(openRef)
	closeRef := b.lower.EphemeralClose(openRef, closeWait)

	closeDependencyAfter(closeRef)

	return stateRef
}
