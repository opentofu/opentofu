// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"iter"
	"maps"
	"sync"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
)

// execRequestTracker is our implementation of [grapheval.RequestTracker] for
// use when processing an execution graph.
//
// An instance of this should be associated with the context passed to the
// graph execution function so that we can use it to produce helpful error
// messages if promise-related errors occur during execution.
type execRequestTracker struct {
	configOracle *eval.ApplyOracle
	graph        *execgraph.Graph

	promiseReqsMu sync.Mutex
	promiseReqs   map[execgraph.PromiseDrivenResultKey]workgraph.RequestID

	graphReqsMu sync.Mutex
	graphReqs   map[workgraph.RequestID]grapheval.RequestInfo
}

var _ grapheval.RequestTracker = (*execRequestTracker)(nil)
var _ execgraph.RequestTrackerWithNotify = (*execRequestTracker)(nil)

func newRequestTracker(graph *execgraph.Graph, ops *execOperations) *execRequestTracker {
	return &execRequestTracker{
		configOracle: ops.configOracle,
		graph:        graph,
		promiseReqs:  make(map[execgraph.PromiseDrivenResultKey]workgraph.RequestID),
		graphReqs:    make(map[workgraph.RequestID]grapheval.RequestInfo),
	}
}

func (e *execRequestTracker) AddRequest(reqID workgraph.RequestID, info grapheval.RequestInfo) {
	e.graphReqsMu.Lock()
	defer e.graphReqsMu.Unlock()
	e.graphReqs[reqID] = info
}

// ActiveRequests implements [grapheval.RequestTracker].
func (e *execRequestTracker) ActiveRequests() iter.Seq2[workgraph.RequestID, grapheval.RequestInfo] {
	return func(yield func(workgraph.RequestID, grapheval.RequestInfo) bool) {
		e.graphReqsMu.Lock()
		activeRequests := map[workgraph.RequestID]grapheval.RequestInfo{}
		maps.Copy(activeRequests, e.graphReqs)
		e.graphReqsMu.Unlock()
		for req, info := range activeRequests {
			if !yield(req, info) {
				return
			}
		}

		// We'll also report any requests that execgraph has announced to us
		// using [execRequestTracker.TrackExecutionGraphRequest].
		//
		// We copy the map of requests here just so we don't hold this lock
		// across yields to the caller. We should only end up in here when
		// we're handling an error so it's okay to spend this extra time.
		e.promiseReqsMu.Lock()
		promiseReqs := make(map[execgraph.PromiseDrivenResultKey]workgraph.RequestID, len(e.promiseReqs))
		maps.Copy(promiseReqs, e.promiseReqs)
		e.promiseReqsMu.Unlock()
		for key, reqID := range promiseReqs {
			info := e.graph.PromiseDrivenRequestInfo(key)
			if !yield(reqID, info) {
				return
			}
		}
	}
}

// TrackExecutionGraphRequest implements [execgraph.RequestTrackerWithNotify].
func (e *execRequestTracker) TrackExecutionGraphRequest(ctx context.Context, key execgraph.PromiseDrivenResultKey, reqID workgraph.RequestID) {
	e.promiseReqsMu.Lock()
	e.promiseReqs[key] = reqID
	e.promiseReqsMu.Unlock()
}
