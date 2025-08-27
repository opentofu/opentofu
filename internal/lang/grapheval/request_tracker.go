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
