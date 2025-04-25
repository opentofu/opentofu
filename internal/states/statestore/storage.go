// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statestore

import (
	"context"
	"iter"

	"github.com/opentofu/opentofu/internal/collections"
)

// Storage is the interface implemented by a state storage implementation,
// which OpenTofu Core uses to persiste and retrieve state information.
//
// To avoid data loss, implementers must ensure that they meet all of the
// requirements described in the description of each method.
//
// The API is designed to allow callers to perform the same action against
// many different keys at once, so that implementations can exploit efficiencies
// from techniques such as batch requests when available in their underlying
// storage APIs.
type Storage interface {
	// Keys enumerates all of the keys currently stored, in no particular order.
	//
	// If a sequence step returns an error then the caller must stop iterating
	// and treat that as an error describing the entire traversal. For example,
	// Storage implementations that need to request data one "page" at a time
	// might succeed in fetching the first page but fail on a subsequent
	// page.
	//
	// In storage implementations that use empty objects as the technique for
	// representing a lock on an object that has not been created yet, the
	// result of this function MUST NOT include the keys associated with those
	// empty objects.
	//
	// Note that there is no overall lock constraining the addition or removal
	// of keys and so the result from this method must be used carefully while
	// taking into account that additional keys could be added concurrently
	// with any downstream work. In particular, the presence of absence of any
	// specific key must not be to make a decision leading to a change
	// unless the caller is also holding a shared or exclusive lock on that key
	// to ensure that it _stays_ absent or present.
	Keys(context.Context) iter.Seq2[Key, error]

	// Lock attempts to acquire a mixture of shared and exclusive locks for
	// zero or more keys.
	//
	// If the returned error nil then all of the requested locks were all
	// acquired and the caller MUST call Unlock for for all of the requested
	// keys at some later point to release the locks. Callers are free to
	// unlock each of the requested keys independently of the others as long
	// as all are unlocked before the program terminates.
	//
	// Implementations are required to allow nested locks to be acquired by
	// different calls to the same [Storage] object, such as by tracking an
	// active lock count for each key and releasing the lock as observed by
	// other clients only when the lock count returns to zero.
	//
	// If any of the requests conflict with already-active locks then the
	// call must block until either all requested objects become available
	// or until the given context is cancelled. If returning due to context
	// cancellation then the returned error must be either [context.Canceled]
	// or [context.DeadlineExceeded] as appropriate for the cancellation type.
	//
	// It is a bug in the caller to include the same key in both the "shared"
	// and "exclusive" sets, or to otherwise request an exclusive lock when
	// already holding a shared lock for the same key. Storage implementations
	// may handle that either by deadlocking, by panicking, or by returning
	// an error.
	//
	// FIXME: After some further design iteration, it doesn't actually seem
	// necessary to support nested locks, so hopefully we can remove that
	// requirement in a later draft.
	Lock(ctx context.Context, shared collections.Set[Key], exclusive collections.Set[Key]) error

	// Unlock releases locks previously acquired using [Storage.Lock].
	//
	// Both shared and exclusive locks can be released using this method, and
	// can be mixed in a single call. There is no requirement that unlock
	// requests be grouped in the same way as earlier successful lock
	// requests.
	//
	// If this function returns an error then the lock state of all of the
	// given keys is unspecified, and so the caller must behave as if all
	// locks were released and should aim to cease further work as soon as
	// possible.
	Unlock(context.Context, collections.Set[Key]) error

	// Read retrieves the values associated with a given set of keys,
	// successfully returning [NoValue] for any that do not currently have
	// an associated value.
	//
	// This may be called only while holding a shared or exclusive lock for
	// all of the given keys, although implementers are not required to enforce
	// that constraint -- reading without acquiring a lock is always a bug in
	// the caller -- but is permitted to enforce it for implementation
	// convenience.
	//
	// If the given set includes more keys than the underlying storage can
	// return in a single request then the implementation must perform
	// multiple requests, using the most efficient strategy available, and
	// then aggregate the results to return.
	Read(context.Context, collections.Set[Key]) (map[Key]Value, error)

	// Write creates, updates, or deletes values associated with a set of
	// keys as described in the given map. A map value of [NoValue] represents
	// a request to completely delete the corresponding key.
	//
	// This may be called only while holding an exclusive lock for all of
	// the given keys, although implementers are not required to enforce that
	// constraint -- writing without acquiring a lock is always a bug in the
	// caller -- but is permitted to enforce it for implementation convenience.
	//
	// If the given map includes more updates than the underlying storage
	// can commit in a single request then the implementation must perform
	// multiple requests. If this function returns an error then it's
	// unspecified which of the given keys have been updated and which have
	// not.
	Write(context.Context, map[Key]Value) error

	// Close is called once the Storage object is no longer needed.
	//
	// Implementations should ensure that any uncommitted changes are persisted
	// to the underlying storage and release any active locks before returning.
	//
	// It's a bug in the caller if any other methods are called after beginning
	// a call to Close.
	Close(context.Context) error
}
