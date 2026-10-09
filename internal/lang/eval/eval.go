// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"
	"iter"
	"maps"
	"sync"

	"github.com/apparentlymart/go-workgraph/workgraph"

	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// checkAll walks the configuration tree starting at the given root module
// instance, collecting diagnostics that describe problems with the
// configuration (but NOT problems outside of the configuration, such as
// apply-time operation failures).
//
// This is really just a wrapper around calling
// [configgraph.ModuleInstance.CheckAll], but it's important to use this
// because it arranges for tracking workgraph request IDs so we can return
// helpful error messages when expression evaluation encounters a
// self-dependency problem.
func checkAll(ctx context.Context, rootModuleInstance evalglue.CompiledModuleInstance) tfdiags.Diagnostics {
	// If the grapheval package detects a self-dependency problem during
	// evaluation then it'll use this tracker to find human-friendly names
	// for all of the requests involved in the error.
	ctx = grapheval.ContextWithRequestTracker(ctx, newWorkgraphRequestTracker())
	return rootModuleInstance.CheckAll(ctx)
}

// workgraphRequestTracker is an awkward piece of glue that helps the
// code in [grapheval] to find user-friendly names for requests in progress
// when it needs to report errors.
//
// The weird indirection here is a compromise to keep this record-tracking
// outside of the main "happy path" code, both because maintainers shouldn't
// need to think about it most of the time and because we then don't need
// to do this request-tracking work unless a grapheval-related error actually
// occurs, since such errors ought to be rare.
type workgraphRequestTracker struct {
	lock           sync.Mutex
	activeRequests map[workgraph.RequestID]grapheval.RequestInfo
}

func newWorkgraphRequestTracker() grapheval.RequestTracker {
	return &workgraphRequestTracker{activeRequests: map[workgraph.RequestID]grapheval.RequestInfo{}}
}

func (w *workgraphRequestTracker) AddRequest(reqID workgraph.RequestID, info grapheval.RequestInfo) {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.activeRequests[reqID] = info
}

// ActiveRequests implements grapheval.RequestTracker.
func (w *workgraphRequestTracker) ActiveRequests() iter.Seq2[workgraph.RequestID, grapheval.RequestInfo] {
	w.lock.Lock()
	defer w.lock.Unlock()
	activeRequests := map[workgraph.RequestID]grapheval.RequestInfo{}
	maps.Copy(activeRequests, w.activeRequests)

	return func(yield func(workgraph.RequestID, grapheval.RequestInfo) bool) {
		for req, info := range activeRequests {
			if !yield(req, info) {
				return
			}
		}
	}
}
