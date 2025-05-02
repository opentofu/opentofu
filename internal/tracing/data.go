// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tracing

import (
	"fmt"
	"iter"

	"go.opentelemetry.io/otel/trace"
)

// This file contains utilities that are not directly related to tracing but
// that are useful for transforming data to include in trace attributes, etc.
//
// These functions all take a span object as their first argument so that
// they can skip performing expensive work when the span is not actually
// recording. The documentation of each function describes how its behavior
// differs when the span is not recording.

// StringSlice takes a sequence of any type that implements [fmt.Stringer]
// and returns a slice containing the results of calling the String method
// on each item in that sequence.
//
// If the given span is not recording then this immediately returns nil
// without consuming the iterator at all.
//
// Use [slices.Values] to use the elements of an existing slice. For example:
//
//	span.SetAttributes(
//	    otelAttr.StringSlice("example", tracing.StringSlice(span, slices.Values(opts.Targets))),
//	)
func StringSlice[E fmt.Stringer](span trace.Span, items iter.Seq[E]) []string {
	if !span.IsRecording() {
		return nil // shortcut for when tracing is not enabled
	}
	var ret []string
	for item := range items {
		ret = append(ret, item.String())
	}
	return ret
}
