// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"
	"iter"

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
	ctx = grapheval.ContextWithRequestTracker(ctx, workgraphRequestTracker{rootModuleInstance})
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
	rootModuleInstance evalglue.CompiledModuleInstance
}

// ActiveRequests implements grapheval.RequestTracker.
func (w workgraphRequestTracker) ActiveRequests() iter.Seq2[workgraph.RequestID, grapheval.RequestInfo] {
	return func(yield func(workgraph.RequestID, grapheval.RequestInfo) bool) {
		// Since we only call into AnnounceAllGraphevalRequests on an error
		// path anyway, we'll make the code in there a little simpler by
		// always announcing every request and just ignore any announcements
		// that come after the consumer of our sequence has asked us to stop.
		// (In practice the caller in grapheval always consumes the full
		// sequence anyway, so this is just for completeness to make sure
		// we always follow the [iter.Seq2] conventions.)
		callerDone := false
		w.rootModuleInstance.AnnounceAllGraphevalRequests(func(reqID workgraph.RequestID, info grapheval.RequestInfo) {
			if callerDone {
				return // caller doesn't want to hear from us anymore
			}
			if reqID == workgraph.NoRequest {
				return // ignore announcements of requests that haven't actually started
			}
			if !yield(reqID, info) {
				callerDone = true // all future announcements will be silently discarded
			}
		})
	}
}
