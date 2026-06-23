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

	// makeDeposedKey is a function provided by the caller for allocating the
	// tracking keys for objects that will become newly-deposed during the
	// apply phase.
	//
	// The implementer is required to make sure that the returned key does not
	// overlap with any already-deposed object for the given resource instance
	// or with any other keys previously returned for the same resource instance
	// address during the same graph-build.
	makeDeposedKey func(addrs.AbsResourceInstance) addrs.DeposedKey
}

// NOTE: There are additional methods for [execGraphBuilder] declared in
// the other files named execgraph_*.go , grouped by what kinds of objects they
// primarily work with.

func newExecGraphBuilder(makeDeposedKey func(addrs.AbsResourceInstance) addrs.DeposedKey) *execGraphBuilder {
	return &execGraphBuilder{
		lower:          execgraph.NewBuilder(),
		makeDeposedKey: makeDeposedKey,
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
