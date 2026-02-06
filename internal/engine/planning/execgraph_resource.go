// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"iter"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
)

////////////////////////////////////////////////////////////////////////////////
// This file contains methods of [execGraphBuilder] that are related to the
// parts of an execution graph that deal with resource instances of all modes.
////////////////////////////////////////////////////////////////////////////////

// SetResourceInstanceFinalStateResult records which result should be treated
// as the "final state" for the given resource instance, for purposes such as
// propagating the result value back into the evaluation system to allow
// downstream expressions to derive from it.
//
// Only one call is allowed per distinct [addrs.AbsResourceInstance] value. If
// two callers try to register for the same address then the second call will
// panic.
func (b *execGraphBuilder) SetResourceInstanceFinalStateResult(addr addrs.AbsResourceInstance, result execgraph.ResourceInstanceResultRef) {
	b.mu.Lock()
	b.lower.SetResourceInstanceFinalStateResult(addr, result)
	b.mu.Unlock()
}

// resourceInstanceFinalStateResult returns the result reference for the given
// resource instance that should previously have been registered using
// [execGraphBuilder.SetResourceInstanceFinalStateResult].
//
// The return type is [execgraph.AnyResultRef] because this is intended for use
// with a general-purpose dependency aggregation node in the graph. The actual
// final state result for the source instance travels indirectly through the
// evaluator rather than directly within the execution graph.
//
// This function panics if a result reference for the given resource instance
// was not previously registered, because that suggests a bug elsewhere in the
// system that caused the construction of subgraphs for different resource
// instances to happen in the wrong order.
func (b *execGraphBuilder) resourceInstanceFinalStateResult(addr addrs.AbsResourceInstance) execgraph.AnyResultRef {
	// TODO: If a caller asks for a resource instance that doesn't yet have
	// a "final result" then we should implicitly insert one that refers
	// to a ResourceInstancePrior result, under the assumption that the upstream
	// thing was a no-op but its state value is still used by something else.
	return b.lower.ResourceInstanceFinalStateResult(addr)
}

// waiterForResourceInstances returns an execgraph result ref which blocks
// until the results of all of the specified resource instances are available.
//
// All of the given instance addresses must be valid to pass to
// [execGraphBuilder.resourceInstanceFinalStateResult]. Refer to that method's
// documentation for information on what exactly that means.
//
// The value of the returned result is not actually meaningful; it's used only
// for its blocking behavior to add additional ordering constraints to an
// execution graph.
//
// The function returned allows callers to ensure any dependency resources
// that stay "open" will not be closed until the given references has completed.
func (b *execGraphBuilder) waiterForResourceInstances(instAddrs iter.Seq[addrs.AbsResourceInstance]) (execgraph.AnyResultRef, registerExecCloseBlockerFunc) {
	var dependencyResults []execgraph.AnyResultRef
	for instAddr := range instAddrs {
		depInstResult := b.resourceInstanceFinalStateResult(instAddr)
		dependencyResults = append(dependencyResults, depInstResult)
	}

	return b.lower.Waiter(dependencyResults...), func(ref execgraph.AnyResultRef) {
		for instAddr := range instAddrs {
			if instAddr.Resource.Resource.Mode == addrs.EphemeralResourceMode {
				b.openEphemeralRefs.Get(instAddr)(ref)
			}
		}
	}
}
