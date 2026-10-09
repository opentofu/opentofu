// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package grapheval

import (
	"iter"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// RequestTracker is implemented by types that know how to provide user-friendly
// descriptions for all active requests in a particular context for error
// reporting purposes.
//
// This is designed to allow delaying at least some of the work required to
// build user-friendly error messages about eval-request-related problems until
// an error actually occurs, because we don't need this information at all in
// the happy path.
//
// Use [ContextWithRequestTracker] to associate a request tracker with a
// [context.Context], and then pass contexts derived from that one to the
// other functions in this package that perform self-dependency and unresolved
// request detection to allow those operations to return better diagnostic
// messages when those situations occur.
type RequestTracker interface {
	// ActiveRequests returns an iterable sequence of all active requests
	// known to the tracker, along with the [RequestInfo] for each one.
	ActiveRequests() iter.Seq2[workgraph.RequestID, RequestInfo]
}

type RequestInfo struct {
	// Name is a short, user-friendly name for whatever this request was trying
	// to calculate.
	Name string

	// SourceRange is an optional source range for something in the
	// configuration that caused this request to be made. Leave this nil
	// for requests that aren't clearly related to a specific element in
	// the given configuration.
	SourceRange *tfdiags.SourceRange
}

// PushActiveRequests is a helper for implementers of
// [RequestTracker.ActiveRequests] that implements its [iter.Seq2]-based
// "pull" API in terms of callback-based "push" implementation that's generally
// more convenient for implementations to offer as they collect information
// across many different child subsystems.
//
// Since this is intended to be called only while handling an error, it makes
// the compromise of presenting the illusion that it's always absorbing the
// entire set reported by the push function but immediately discarding any
// that show up after the sequence is no longer being consumed.
func PushActiveRequests(push func(announce func(workgraph.RequestID, RequestInfo))) iter.Seq2[workgraph.RequestID, RequestInfo] {
	return func(yield func(workgraph.RequestID, RequestInfo) bool) {
		keepGoing := true
		push(func(reqID workgraph.RequestID, info RequestInfo) {
			if !keepGoing {
				// We just ignore any calls that arrive after the yield
				// function has returned false, since nobody is reading
				// the sequence anymore in that case.
				return
			}
			keepGoing = yield(reqID, info)
		})
	}
}
