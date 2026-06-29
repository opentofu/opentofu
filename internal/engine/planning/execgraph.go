// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
)

// execGraphBuilder is a legacy leftover of an earlier version of this component
// where execution graph construction ran concurrently with other planning work.
//
// That's no longer true and so we'll probably remove or further simplify this
// eventually. Only [buildExecutionGraph] should instantiate objects of this
// type, and outside callers should rely only on [buildExecutionGraph].
type execGraphBuilder struct {
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

func buildExecutionGraph(
	objs *resourceInstanceObjects,
	effectiveReplaceOrders addrs.Map[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder],
	makeDeposedKey func(addrs.AbsResourceInstance) addrs.DeposedKey,
) *execgraph.Graph {
	// TODO: This was originally built around a separate [execGraphBuilder]
	// type because we were building the execution graph concurrently with
	// other planning work, and so it was convenient to have a central object
	// to hold the necessary mutex and otherwise help coordinate between
	// multiple callers.
	//
	// We no longer use that structure and instead just build the execution
	// using normal sequential code after the rest of the planning work is
	// complete, and so it's debatable whether we even need this
	// [execGraphBuilder] type anymore, but the main functionality currently
	// lives as methods of this type and so we'll keep it for now until the
	// shape of this part of the system is feeling more settled and then we
	// can decide whether to simplify further. But for now let's have the
	// rest of this package pretend that [execGraphBuilder] doesn't exist so
	// that we can refactor this more easily later.

	egb := &execGraphBuilder{
		lower:          execgraph.NewBuilder(),
		makeDeposedKey: makeDeposedKey,
	}
	egb.AddResourceInstanceObjectSubgraphs(
		objs,
		effectiveReplaceOrders,
	)
	return egb.lower.Finish()
}
