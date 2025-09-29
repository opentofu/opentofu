// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package grapheval

import (
	"context"
	"fmt"
	"iter"
	"slices"
	"strings"

	"github.com/apparentlymart/go-workgraph/workgraph"
	hcl "github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// DiagnosticsForWorkgraphError transforms an error returned by a call to
// [workgraph.Promise.Await], describing a problem that occurred in the
// active request graph, into user-facing diagnostic messages describing the
// problem.
//
// This function can only produce a user-friendly result when the given context
// contains a request tracker as arranged by [ContextWithRequestTracker], and
// that tracker is able to report all of the requests involved in the problem.
// If that isn't true then the diagnostic messages will lack important
// information and will report that missing information as being a bug in
// OpenTofu, because we should always be tracking requests correctly.
func DiagnosticsForWorkgraphError(ctx context.Context, err error) tfdiags.Diagnostics {
	tracker := RequestTrackerFromContext(ctx)

	if tracker == nil {
		// In this case we must return lower-quality error messages because
		// we don't have any way to name the affected requests. This is
		// primarily for internal callers like unit tests; we should avoid
		// getting here in any case where the result is being returned to
		// end-users.
		return diagnosticsForWorkgraphErrorUntracked(err)
	}

	// In the most happy case we have an active request tracker and so we
	// should be able to describe the individial requests that were impacted
	// by this problem.
	return diagnosticsForWorkgraphErrorTracked(err, tracker)
}

func diagnosticsForWorkgraphErrorTracked(err error, tracker RequestTracker) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	switch err := err.(type) {
	case workgraph.ErrSelfDependency:
		// This is the only case that (probably) doesn't represent a bug in
		// OpenTofu: we will get in here if OpenTofu is tracking everything
		// correctly but the configuration contains an expression that depends
		// on its own result, directly or indirectly.
		reqInfos := collectRequestsInfo(slices.Values(err.RequestIDs), tracker)
		reqDescs := make([]string, 0, len(reqInfos))
		for _, reqID := range err.RequestIDs {
			desc := "<unknown object> (failing to report this is a bug in OpenTofu)"
			if info := reqInfos[reqID]; info != nil {
				if info.SourceRange != nil {
					desc = fmt.Sprintf("%s (%s)", info.Name, info.SourceRange.StartString())
				} else {
					desc = info.Name
				}
			}
			reqDescs = append(reqDescs, desc)
		}
		slices.Sort(reqDescs)

		var detailBuf strings.Builder
		detailBuf.WriteString("The following objects in the configuration form a dependency cycle, so there is no valid order to evaluate them in:\n")
		for _, desc := range reqDescs {
			fmt.Fprintf(&detailBuf, "  - %s\n", desc)
		}

		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Self-referential expressions",
			strings.TrimSpace(detailBuf.String()),
		))
	case workgraph.ErrUnresolved:
		reqName := "<unknown request>"
		var sourceRange *hcl.Range

		reqInfos := collectRequestsInfo(oneSeq(err.RequestID), tracker)
		if reqInfo := reqInfos[err.RequestID]; reqInfo != nil {
			reqName = reqInfo.Name
			if reqInfo.SourceRange != nil {
				sourceRange = reqInfo.SourceRange.ToHCL().Ptr()
			}
		}

		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Configuration evaluation failed",
			Detail:   fmt.Sprintf("During configuration evaluation, %q was left unresolved. This is a bug in OpenTofu.", reqName),
			Subject:  sourceRange,
		})
	default:
		// We should not get here because the two cases above cover everything
		// that package workgraph should return.
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Evaluation failed",
			"Configuration evaluation failed for an unknown reason. This is a bug in OpenTofu.",
		))
	}
	return diags
}

func diagnosticsForWorkgraphErrorUntracked(err error) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	switch err.(type) {
	case workgraph.ErrUnresolved:
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Evaluation failed",
			"An unexpected problem prevented complete evaluation of the configuration. This is a bug in OpenTofu.",
		))
	case workgraph.ErrSelfDependency:
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Self-referential expressions",
			"The configuration contains expressions that form a dependency cycle. Unfortunately, a bug in OpenTofu prevents reporting the affected expressions.",
		))
	default:
		// We should not get here because the two cases above cover everything
		// that package workgraph should return.
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Evaluation failed",
			"Configuration evaluation failed for an unknown reason. This is a bug in OpenTofu.",
		))
	}
	return diags
}

// collectRequestsInfo collects a [RequestInfo] value for each of the given
// request IDs that is known to the given tracker, or reports nil for any
// request ID that is not known to the tracker.
func collectRequestsInfo(requestIDs iter.Seq[workgraph.RequestID], tracker RequestTracker) map[workgraph.RequestID]*RequestInfo {
	ret := make(map[workgraph.RequestID]*RequestInfo)
	for requestID := range requestIDs {
		ret[requestID] = nil
	}
	for requestID, info := range tracker.ActiveRequests() {
		if _, ok := ret[requestID]; ok {
			ret[requestID] = &info
		}
	}
	return ret
}

// FIXME: This is a placeholder for what's proposed here but not yet accepted
// at the time of writing: https://github.com/golang/go/issues/68947
func oneSeq[T any](value T) iter.Seq[T] {
	return func(yield func(T) bool) {
		yield(value)
	}
}
