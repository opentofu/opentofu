// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"sync"

	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// CheckGroup has a similar purpose to [sync.WaitGroup], but specialized for
// ergonomic implementation of the "CheckAll" methods we use for our tree
// walks to collect all results.
//
// It uses the [workgraph] facilities so we can detect if multiple check
// requests would end up blocking on one another and return error diagnostics
// in that case.
//
// The expected pattern is something like this:
//
//	var cg CheckGroup
//	cg.CheckValuer(ctx, thisObject.SomeValuer)
//	cg.CheckChild(ctx, thisObject.SomeChildObject)
//	return cg.Complete(ctx)
//
// The Complete method then waits for all of the requested checks to complete
// and returns all of the diagnostics collected across them all.
type CheckGroup struct {
	wg sync.WaitGroup

	diags tfdiags.Diagnostics // must lock mu to access, until wg.Wait returns.
	mu    sync.Mutex
}

func (g *CheckGroup) CheckChild(ctx context.Context, child allChecker) {
	g.wg.Go(func() {
		diags := child.CheckAll(grapheval.ContextWithNewWorker(ctx))
		g.mu.Lock()
		g.diags = g.diags.Append(diags)
		g.mu.Unlock()
	})
}

func (g *CheckGroup) CheckValuer(ctx context.Context, v exprs.Valuer) {
	g.wg.Go(func() {
		// We use Value to make sure we're running the same codepath that
		// normal evaluation would use, but we only care about the diags.
		_, diags := v.Value(grapheval.ContextWithNewWorker(ctx))
		g.mu.Lock()
		g.diags = g.diags.Append(diags)
		g.mu.Unlock()
	})
}

func (g *CheckGroup) CheckDiagsFunc(ctx context.Context, f func(ctx context.Context) tfdiags.Diagnostics) {
	g.wg.Go(func() {
		diags := f(grapheval.ContextWithNewWorker(ctx))
		g.mu.Lock()
		g.diags = g.diags.Append(diags)
		g.mu.Unlock()
	})
}

// Await is for situations where we must wait for some other worker-blocking
// operation to complete to decide what other Check* calls to make. The
// given callback can block on arbitrary workgraph-coordinated operations
// but should eventually make zero or more calls to Check* methods on the
// same [checkGroup] before it returns.
func (g *CheckGroup) Await(ctx context.Context, cb func(ctx context.Context)) {
	g.wg.Go(func() {
		// We give the waiter its own worker since it may run concurrently
		// with other Awaits or with Check* calls.
		cb(grapheval.ContextWithNewWorker(ctx))
	})
}

// Complete blocks until all previous calls to Check* methods have completed
// and then returns all of the aggregated diagnostics.
//
// After calling Complete the [checkGroup] is closed and must not be used
// anymore.
func (g *CheckGroup) Complete(_ context.Context) tfdiags.Diagnostics {
	g.wg.Wait()
	return g.diags
}

// allChecker is implemented by types representing tree nodes that know how
// to check themselves and everything beneath them.
type allChecker interface {
	CheckAll(ctx context.Context) tfdiags.Diagnostics
}
