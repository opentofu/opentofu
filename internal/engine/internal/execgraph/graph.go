// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states"
)

type Graph struct {
	// Overall "graph" is modelled as a collection of tables representing
	// different kinds of nodes, and then the actual graph relationships are
	// modeled as [ResultRef] or [AnyResultRef] values that are all really
	// just indices into these tables.

	//////// Constants saved directly in the graph
	// The tables in this section are values that are decided during the
	// planning phase and need not be recalculated during the apply phase,
	// and so we just store them directly.

	// constantVals is the table of constant values that are to be saved
	// directly inside the execution graph.
	constantVals []cty.Value
	// providerAddrs is the table of provider addresses that are to be saved
	// directly inside the execution graph.
	providerAddrs []addrs.Provider

	//////// ApplyOracle queries
	// The tables in this section represent requests for information from
	// the configuration evaluation system via its ApplyOracle API.

	// desiredStateRefs is the table of references to resource instances from
	// the desired state.
	desiredStateRefs []addrs.AbsResourceInstance
	// providerInstConfigRefs is the table of references to provider instance
	// configuration values.
	providerInstConfigRefs []addrs.AbsProviderInstanceCorrect

	//////// Prior state queries
	// The tables in this section represent requests for information from
	// the prior state.

	// priorStateRefs is the table of references to resource instance objects
	// from the prior state.
	priorStateRefs []resourceInstanceStateRef

	//////// The actual side-effects
	// The tables in this section deal with the main side-effects that we're
	// intending to perform, and modelling the interactions between them.
	//
	// These are the only graph nodes that can directly depend on results from
	// other graph nodes. Everything in the other sections above is fetching
	// data from outside of the apply engine, although those which interact
	// with the ApplyOracle will often depend indirectly on results in this
	// section where the configuration defines the desired state for one
	// resource instance in terms of the final state of another resource
	// instance.

	// ops are the actual operations -- functions with side-effects --
	// that are the main purpose of the execution graph. Operations can
	// depend on each other and on constant values or state references.
	ops []operationDesc
	// waiters are nodes that just express a dependency on the work that
	// produces some other results even though the actual value of the
	// result isn't needed. For example, this can be used to describe
	// what other work needs to complete before a provider instance is closed.
	//
	// Although it's not actually enforced by the model, it's only really useful
	// to add _operation_ results to "waiter" nodes, because operations are
	// how we model side-effects that we might need to wait for completion of.
	waiters [][]AnyResultRef
	// resourceInstanceResults are "sink" nodes that capture references to
	// the "final state" results for desired resource instances that are
	// subject to changes in this graph, allowing the resulting values to
	// propagate back into the evaluation system so that downstream resource
	// instance configurations can be derived from them.
	//
	// Due to the behavior of the concurrently-running expression evaluation
	// system, there's an effective implied dependency edge between results
	// captured in here and the entries in desiredStateRefs for any resource
	// instances whose configuration is derived from the result of an entry
	// in this map. However, the execution graph is not supposed to rely on
	// those implied edges for correct execution order: the "final plan"
	// operation for each resource instance should also directly depend on
	// the results of any resource instances that were identified as
	// resource-instance-graph dependencies during the planning process.
	resourceInstanceResults addrs.Map[addrs.AbsResourceInstance, ResultRef[*states.ResourceInstanceObjectFull]]
}

type resourceInstanceStateRef struct {
	ResourceInstance addrs.AbsResourceInstance
	DeposedKey       states.DeposedKey
}
