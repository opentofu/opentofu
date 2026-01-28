// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"context"
	"fmt"
	"sync"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type CompiledGraph struct {
	// steps is the main essence of a compiled graph: a series of functions
	// that we'll run all at once, one goroutine each, and then wait until
	// they've all returned something.
	//
	// In practice these functions will typically depend on one another
	// indirectly through [workgraph.Promise] values, but it's up to the
	// compiler to arrange for the necessary data flow while it's building
	// these compiled operations. Execution is complete once all of these
	// functions have returned.
	steps []compiledGraphStep

	// resourceInstanceValues provides a function for each resource instance
	// that was registered as a "sink" during graph building which blocks
	// until the final state for that resource instance is available and then
	// returns the object value to represent the resource instance in downstream
	// expression evaluation.
	resourceInstanceValues addrs.Map[addrs.AbsResourceInstance, func(ctx context.Context) cty.Value]

	// cleanupWorker is the workgraph worker that is initially responsible
	// for resolving all of the workgraph requests created by the compiler,
	// but in the happy path they should all be gradually delegated to
	// workers created by the functions in "ops", leaving this worker
	// responsible for nothing.
	//
	// We track this here just so that if, for some exceptional reason, the
	// CompiledGraph object gets garbage collected before everything has been
	// handled then all of the remaining requests will get resolved with an
	// error to ensure that everything gets to unwind and thus we won't leak
	// goroutines.
	cleanupWorker *workgraph.Worker
}

// compiledGraphStep is a single "step" from a [CompiledGraph].
//
// [CompiledGraph.Execute] executes all of the steps concurrently in a
// separate goroutine each, and so a compiledGraphStep function should start
// by establishing a new [workgraph.Worker] to represent whatever work it's
// going to do, so that the system can detect when a step tries to depend
// on its own result, directly or indirectly.
type compiledGraphStep func(ctx context.Context) tfdiags.Diagnostics

// Execute performs all of the work described in the execution graph in a
// suitable order, returning any diagnostics that operations might return
// along the way.
//
// If there are resource instance operations in the graph (which is typical for
// any useful execution graph) then typically the evaluation system should
// be running concurrently and be taking resource instance results from
// calls to [CompiledGraph.ResourceInstanceValue] so that the graph execution
// and evaluation system can collaborate to drive the execution process forward
// together.
func (c *CompiledGraph) Execute(ctx context.Context) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	var diagsMu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(len(c.steps))
	for idx, step := range c.steps {
		if step == nil {
			diags = diags.Append(fmt.Errorf("execution graph compiled step %d is nil function", idx))
			return diags
		}
		go func() {
			opDiags := step(ctx)
			diagsMu.Lock()
			diags = diags.Append(opDiags)
			diagsMu.Unlock()
			wg.Done()
		}()
	}
	wg.Wait()

	return diags
}

// ResourceInstanceValue blocks until after changes have been applied for the
// given resource instance address and then returns a [cty.Value] that should
// represent that resource instance in downstream expression evaluation.
//
// Calls to this method should run concurrently with a call to
// [CompiledGraph.Execute] because otherwise the operations that generate the
// final state for resource instances will not run and thus this will block
// indefinitely waiting for results that will never arrive.
func (c *CompiledGraph) ResourceInstanceValue(ctx context.Context, addr addrs.AbsResourceInstance) cty.Value {
	getter, ok := c.resourceInstanceValues.GetOk(addr)
	if !ok {
		// If we get asked for a resource instance address that wasn't involved
		// in the plan then we'll assume it was excluded from the plan by
		// something like the -target option or deferred actions, and so we'll
		// just return a completely-unknown placeholder to let the rest of the
		// evaluation proceed. This should be valid as long as the planning
		// phase made valid and consistent decisions about what to exclude,
		// such that if a particular resource instance is excluded then any
		// other resource or provider instance that depends on it must also be
		// excluded.
		return cty.DynamicVal
	}
	return getter(ctx)
}

// trackWorkgraphRequest attempts to notify an upstream about a workgraph
// request we've started as part of producing the result for the given
// reference.
//
// This is here to help an execgraph caller -- typically the applying engine --
// to produce a more helpful error message if any promise-related errors occur
// during execution graph processing. Calls to this are not used when execution
// is successful, but unfortunately we nonetheless need to send this information
// anyway just in case a related error occurs downstream somewhere.
//
// In order to benefit from this the execgraph caller must use []
func trackWorkgraphRequest(ctx context.Context, operationIdx int, reqID workgraph.RequestID) {
	reqTracker := grapheval.RequestTrackerFromContext(ctx)
	if reqTracker == nil {
		return
	}
	// Unfortunately this relies on the "optional extension interface" pattern,
	// so we have no compile-time checking that the applying engine implements
	// this correctly. This is a pragmatic concession since we should only
	// need this information in "should never happen" error cases in order to
	// produce more helpful diagnostic information, since there should not be
	// any promise-related errors when handling a correctly-constructed
	// execution graph.
	withNotify, ok := reqTracker.(RequestTrackerWithNotify)
	if !ok {
		return
	}
	key := PromiseDrivenResultKey{operationIdx: operationIdx}
	withNotify.TrackExecutionGraphRequest(ctx, key, reqID)
}

// RequestTrackerWithNotify is an optional extension of [grapheval.RequestTracker]
// which must be implemented by the request tracker passed in
// [CompiledGraph.Execute]'s [context.Context] argument if the request tracker
// needs to be aware of workgraph requests made as part of resolving results
// during execution graph processing.
//
// If the given context has no request tracker at all, or if the request tracker
// does not implement this extension interface, then no notifications will be
// delivered but execution is otherwise unaffected.
type RequestTrackerWithNotify interface {
	grapheval.RequestTracker
	TrackExecutionGraphRequest(ctx context.Context, key PromiseDrivenResultKey, reqID workgraph.RequestID)
}

// PromiseDrivenResultKey is an opaque representation of a [ResultRef] whose
// result is produced using [grapheval] and [workgraph] mechanisms, which
// we use in [RequsetTrackerWithNotify] to give a caller just enough information
// to selectively request more detail from a source graph only if necessary
// to report an error.
type PromiseDrivenResultKey struct {
	// for now operations are the only kind of result we track in this way,
	// and so this just directly tracks the operation index.
	operationIdx int
}
