// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
)

// execGraphBuilder is a higher-level wrapper around [execgraph.Builder] that
// is tailored to the needs of the planning engine.
//
// Specifically:
//   - Its exported methods that add to or modify the graph are all
//     concurrency-safe, for convenient use during the concurrent planning work
//     driven by the evaluator.
//   - It keeps track of certain "singleton" collections of graph nodes that
//     different parts of the planning engine all need to agree on for the
//     execution graph to be correct, such as ensuring there's only one open
//     and one close operation per distinct provider instance address.
//   - Many of its methods can potentially add multiple operations to the graph
//     at once, to let the planning engine work at a higher level of abstraction
//     than just the individual raw operation types. The lower-level
//     [execgraph.Builder] instead directly matches the abstraction level of
//     [execgraph.Operations].
type execGraphBuilder struct {
	// mu must be locked while accessing any of the other fields.
	mu sync.Mutex

	// lower is the lower-level graph builder that this utility is built in
	// terms of.
	lower *execgraph.Builder

	// During construction we treat certain items as singletons so that
	// we can do the associated work only once while providing it to
	// multiple callers, and so these maps track those singletons but
	// we throw these away after building is complete because the graph
	// becomes immutable at that point.
	resourceInstAddrRefs addrs.Map[addrs.AbsResourceInstance, execgraph.ResultRef[addrs.AbsResourceInstance]]
}

// NOTE: There are additional methods for [execGraphBuilder] declared in
// the other files named execgraph_*.go , grouped by what kinds of objects they
// primarily work with.

func newExecGraphBuilder() *execGraphBuilder {
	return &execGraphBuilder{
		lower:                execgraph.NewBuilder(),
		resourceInstAddrRefs: addrs.MakeMap[addrs.AbsResourceInstance, execgraph.ResultRef[addrs.AbsResourceInstance]](),
	}
}

// Finish returns the graph that has been built, which is then immutable.
//
// After calling this function the execGraphBuilder is invalid and must not be
// used anymore.
func (b *execGraphBuilder) Finish() *execgraph.Graph {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lower.Finish()
}

// makeCloseBlocker is a helper used by [execGraphBuilder] methods that produce
// open/close node pairs.
//
// Callers MUST hold a lock on b.mu throughout any call to this method, AND
// when calling the returned callback.
func (b *execGraphBuilder) makeCloseBlocker() (execgraph.AnyResultRef, func(execgraph.AnyResultRef)) {
	waiter, lowerRegister := b.lower.MutableWaiter()
	registerFunc := func(ref execgraph.AnyResultRef) {
		lowerRegister(ref)
	}
	return waiter, registerFunc
}
