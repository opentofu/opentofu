// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lifecycle

import (
	"iter"
	"maps"
	"sync"

	"github.com/opentofu/opentofu/internal/collections"
)

// CompletionTracker is a synchronization utility that keeps a record of the
// completion of various items and allows various different goroutines to wait
// for the completion of different subsets of the items.
//
// "Items" can be of any comparable type, but the design intention is that a
// caller will define its own types to represent the different kinds of work
// it needs to track.
type CompletionTracker[T comparable] struct {
	mu        sync.Mutex
	completed collections.Set[T]
	waiters   collections.Set[*completionWaiter[T]]
}

type completionWaiter[T comparable] struct {
	pending collections.Set[T]
	ch      chan<- struct{}
}

// NewCompletionTracker returns a new [CompletionTracker] that initially
// has no waiters and no completed items.
func NewCompletionTracker[T comparable]() *CompletionTracker[T] {
	return &CompletionTracker[T]{
		completed: collections.NewSet[T](),
		waiters:   collections.NewSet[*completionWaiter[T]](),
	}
}

// ItemComplete returns true if the given item has already been reported
// as complete using [CompletionTracker.ReportCompletion].
//
// A complete item can never become incomplete again, but if this function
// returns false then a concurrent goroutine could potentially report the
// item as complete before the caller acts on that result.
func (t *CompletionTracker[T]) ItemComplete(item T) bool {
	t.mu.Lock()
	_, ret := t.completed[item]
	t.mu.Unlock()
	return ret
}

// NewWaiterFor returns an unbuffered channel that will be closed once all
// of the addresses in the given seqence have had their completion reported
// using [CompletionTracker.ReportCompletion].
//
// No items will be sent to the channel.
//
// For callers that would just immediately block waiting for the given channel
// to be closed (without using it as part of a larger "select" statement),
// consider using the simpler [CompletionTracker.WaitFor] instead.
func (t *CompletionTracker[T]) NewWaiterFor(waitFor iter.Seq[T]) <-chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()

	ch := make(chan struct{})
	waiter := &completionWaiter[T]{
		pending: collections.NewSet[T](),
		ch:      ch,
	}
	for item := range waitFor {
		if t.completed.Has(item) {
			continue // ignore any already-completed items
		}
		waiter.pending[item] = struct{}{}
	}

	if len(waiter.pending) == 0 {
		// If we didn't find any addresses that were not already completed
		// then we'll just close the channel immediately before we return,
		// and not track the waiter at all.
		close(ch)
		return ch
	}

	// If we have at least one item to wait for then we'll remember this
	// new tracker so we can reconsider it each time something has its
	// completion reported.
	t.waiters[waiter] = struct{}{}
	return ch
}

// WaitFor blocks until all of the addresses in the given set have had their
// completion reported using [CompletionTracker.ReportCompletion].
//
// This is a convenience wrapper for [CompletionTracker.NewWaiterFor] that
// just blocks until the returned channel is closed.
func (t *CompletionTracker[T]) WaitFor(waitFor iter.Seq[T]) {
	ch := t.NewWaiterFor(waitFor)
	for range ch {
		// just block until the channel is closed
	}
}

// ReportCompletion records the completion of the given item and signals
// any waiters for which it was the last remaining pending item.
func (t *CompletionTracker[T]) ReportCompletion(of T) {
	t.mu.Lock()
	t.completed[of] = struct{}{}
	for waiter := range t.waiters {
		delete(waiter.pending, of)
		if len(waiter.pending) == 0 {
			// nothing left for this waiter to wait for
			close(waiter.ch)
			delete(t.waiters, waiter)
		}
	}
	t.mu.Unlock()
}

// PendingItems returns a set of all of the items that are pending for at
// least one waiter at the time of the call.
//
// This is mainly to allow detection and cleanup of uncompleted work: once
// a caller thinks that all work ought to have completed it can call this
// function and should hopefully receive an empty set. If not, it can report
// an error about certain items being left unresolved and optionally make
// synthetic calls to [CompletionTracker.ReportCompletion] to cause all of the
// remaining waiters to be unblocked.
//
// The result is a fresh set allocated for each call, so the caller is free
// to modify that set without corrupting the internal state of the
// [CompletionTracker].
func (t *CompletionTracker[T]) PendingItems() collections.Set[T] {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.waiters) == 0 {
		return nil
	}
	ret := collections.NewSet[T]()
	for waiter := range t.waiters {
		maps.Copy(ret, waiter.pending)
	}
	return ret
}
