// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package grapheval

import (
	"context"

	"github.com/apparentlymart/go-workgraph/workgraph"
)

// ContextWithWorker returns a child of the given context that is associated
// with the given [workgraph.Worker].
//
// This is a very low-level API for use by code that is interacting directly
// with the [workgraph] package API. Callers should prefer to use the
// higher-level wrappers in this package whenever possible, because they
// manage context-based worker tracking automatically on the caller's behalf.
func ContextWithWorker(parent context.Context, worker *workgraph.Worker) context.Context {
	return context.WithValue(parent, workerContextKey, worker)
}

// ContextWithNewWorker is like [ContextWithWorker] except that it internally
// creates a new worker and associates that with the returned context.
//
// This is a good way to create the top-level context needed for first entry
// into a call graph that relies on the self-reference-detecting functions
// elsewhere in this package. Those functions will then create other workers
// as necessary.
func ContextWithNewWorker(parent context.Context) context.Context {
	return ContextWithWorker(parent, workgraph.NewWorker())
}

// WorkerFromContext returns a pointer to the [workgraph.Worker] associated
// with the given context, or panics if the context has no worker.
func WorkerFromContext(ctx context.Context) *workgraph.Worker {
	worker, ok := ctx.Value(workerContextKey).(*workgraph.Worker)
	if !ok {
		panic("no worker handle in this context")
	}
	return worker
}

// ContextWithRequestTracker returns a child of the given context that is
// associated with the given [RequestTracker].
//
// Pass promises derived from the result to other functions in this package that
// perform self-reference and unresolved request detection to improve the
// error messages returned when those error situations occur.
func ContextWithRequestTracker(parent context.Context, tracker RequestTracker) context.Context {
	return context.WithValue(parent, trackerContextKey, tracker)
}

// RequestTrackerFromContext returns the request tracker associated with the
// given context, or nil if there is no request tracker.
func RequestTrackerFromContext(ctx context.Context) RequestTracker {
	tracker, ok := ctx.Value(trackerContextKey).(RequestTracker)
	if !ok {
		return nil
	}
	return tracker
}

type contextKey rune

const workerContextKey = contextKey('W')
const trackerContextKey = contextKey('T')
