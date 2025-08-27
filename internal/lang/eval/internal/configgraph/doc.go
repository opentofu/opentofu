// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package configgraph contains the unexported implementation details of
// package eval.
//
// Package eval offers an API focused on what external callers need to
// implement specific operations like the plan and apply phases, while
// hiding the handling of language features that get treated equivalently
// regardless of phase. This package is the main place that such handling is
// hidden.
//
// All functions in this package which take context.Context objects as their
// first argument require a context that's derived from a call to
// [grapheval.ContextWithWorker], and in non-test situations _also_ one
// derived from a call to [grapheval.ContextWithRequestTracker] to allow
// identifying failed evaluation requests in error messages.
package configgraph
