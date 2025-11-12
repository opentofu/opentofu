// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tracing

import (
	"context"
	"iter"
	"maps"
	"runtime"
	"sync"
	"testing"
)

// ContextProbe is a testing helper to allow tests to check whether
// [context.Context] values are being propagated correctly to various downstream
// functions where context value continuity is important for certain
// functionality, like tracing. (It's in this package because tracing is our
// primary motivation, but could potentially be used for other
// context-value-related situations too.)
//
// To use it, first call [NewContextProbe] from the test that wants to verify
// propagation, which returns both a [ContextProbe] and a [context.Context]
// that carries a value referring to it. Then in the function whose
// functionality requires context values to reach it, call [ContextProbeReport]
// with that function's own local context to notify any active context probe
// that the function was called. Finally, at the end of the test call
// [ContextProbe.ExpectReportsFrom] with all of the functions that the test
// expects should have been able to successfully call [ContextProbeReport].
type ContextProbe struct {
	calls map[string]struct{}
	mu    sync.Mutex
}

type contextProbeKeyType int

const contextProbeKey = contextProbeKeyType(0)

// NewContextProbe creates a new [ContextProbe] and a new context (child of base)
// that is bound to it, so that [ContextProbeReport] with that context would
// record the call in the probe.
func NewContextProbe(t testing.TB, base context.Context) (context.Context, *ContextProbe) {
	if existing := base.Value(contextProbeKey); existing != nil {
		// We can only have one at a time so this is likely to be a programming
		// error in the calling test, and so we'll report it explicitly rather
		// than just quietly doing something confusing.
		t.Fatal("base context already has a ContextProbe")
	}
	probe := &ContextProbe{
		calls: make(map[string]struct{}),
	}
	ctx := context.WithValue(base, contextProbeKey, probe)
	return ctx, probe
}

func (p *ContextProbe) report(f *runtime.Func) {
	p.mu.Lock()
	p.calls[f.Name()] = struct{}{}
	p.mu.Unlock()
}

// ExpectReportsFrom generates test errors (but does not terminate the test)
// if any of the given function names have not yet been reported by a
// call to [ContextProbeReport].
//
// Returns true if no errors were generated, or false if at least one error
// was generated.
func (p *ContextProbe) ExpectReportsFrom(t testing.TB, names ...string) bool {
	ret := true
	for _, name := range names {
		if _, called := p.calls[name]; !called {
			t.Error("tracing.ContextProbeReport was not called by " + name)
			ret = false
		}
	}
	return ret
}

// FunctionsReported returns an interable sequence of all of the functions
// that have called [ContextProbeReport] so far, in no particular order.
//
// Most tests should prefer to use [ContextProbe.ExpectReportsFrom] so that
// they don't get broken by reports intended for use by other tests, but
// this can be useful as a temporary addition to a test for debugging purposes,
// or to find out how the Go runtime describes a particular function of
// interest.
func (p *ContextProbe) FunctionsReported() iter.Seq[string] {
	return maps.Keys(p.calls)
}

// ContextProbeReport notifies the [ContextProbe] in the given context, if any,
// that its caller has been called.
//
// skipFrames is the number of callers to skip when deciding the name of the
// caller. Zero means to record the direct caller of ContextProbeReport.
//
// When called with a context that does not have a [ContextProbe] this does
// only the minimum work required to determine that there is no probe and
// immediately returns. The overhead is small, but there is still some overhead
// and so this function should not be called from functions used in tight loops
// but is okay to leave in normal codepaths otherwise.
func ContextProbeReport(ctx context.Context, skipFrames int) {
	probe, ok := ctx.Value(contextProbeKey).(*ContextProbe)
	if !ok {
		return // fast return path for the no-probe case, to minimize overhead
	}

	callerPc, _, _, ok := runtime.Caller(skipFrames + 1)
	if !ok {
		return
	}
	caller := runtime.FuncForPC(callerPc)
	if caller == nil {
		return
	}

	probe.report(caller)
}
