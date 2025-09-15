// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lifecycle

import (
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

func TestCompletionTracker(t *testing.T) {
	ctx := t.Context()

	// This test would be a good candidate for the testing/synctest package,
	// but at the time of writing it that package hasn't been stablized yet.
	//
	// Using testing/synctest would make it possible to use synctest.Wait()
	// to be sure that the waiter goroutine has had a chance to write to
	// the "complete" flag before we test it, and so it would not need to
	// be an atomic.Bool anymore and our test that completing only a subset
	// of the items doesn't unblock would be reliable rather than best-effort.

	// We'll use strings as the tracking keys here for simplicity's sake,
	// but the intention is that real callers of this would define their
	// own types to represent different kinds of trackable objects.
	tracker := NewCompletionTracker[string]()
	tracker.ReportCompletion("completed early")

	var complete atomic.Bool
	waitCh := tracker.NewWaiterFor(slices.Values([]string{
		"completed early",
		"completed second",
		"completed last",
	}))
	waiterExitCh := make(chan struct{}) // closed once the goroutine below has finished waiting
	go func() {
		select {
		case <-waitCh:
			complete.Store(true)
			t.Log("waiting channel was closed")
		case <-ctx.Done():
			// We'll get here if the test finishes before waitCh is closed,
			// suggesting that something went wrong. We'll just return to
			// avoid leaking this goroutine, since the surrounding test has
			// presumably already failed anyway.
		}
		close(waiterExitCh)
	}()

	if complete.Load() {
		t.Fatal("already complete before we resolved any other items")
	}
	t.Log("resolving the second item")
	tracker.ReportCompletion("completed second")
	// NOTE: The following is best effort as long as we aren't using
	// test/synctest, because we can't be sure that the waiting goroutine
	// has been scheduled long enough to reach the complete.Store(true).
	time.Sleep(10 * time.Millisecond)
	if complete.Load() {
		t.Fatal("already complete before we resolved the final item")
	}
	t.Log("resolving the final item")
	tracker.ReportCompletion("completed last")
	<-waiterExitCh // wait for the waiter goroutine to exit
	if !complete.Load() {
		t.Fatal("not complete even though all items are resolved")
	}

	// creating a waiter for items that have all already completed should
	// return a channel that's already closed.
	alreadyCompleteWaitCh := tracker.NewWaiterFor(slices.Values([]string{
		"completed early",
		"completed last",
	}))
	select {
	case <-alreadyCompleteWaitCh:
		// the expected case
	case <-time.After(1 * time.Second):
		t.Fatal("already-completed waiter channel was not immediately closed")
	case <-ctx.Done():
		// test has somehow already exited?! (this should not be possible)
	}
}
