// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"

	"github.com/opentofu/opentofu/internal/states/statekeys"
	"github.com/opentofu/opentofu/internal/states/statestore"
)

type stateLocker struct {
	// storage is the state storage implementation that we'll acquire locks
	// through.
	storage statestore.Storage

	// activeLocks tracks all of the state storage locks that the system thinks
	// it currently holds.
	//
	// The value is true for an exclusive lock or false for a shared lock.
	// The absense of an element represents no lock at all.
	//
	// If "poisoned" is set then activeLocks is known to be in an inconsistent
	// state e.g. due to a failure of the underlying storage, in which case
	// all future lock requests will fail to encourage the system to shut down
	// quickly. (The map could also potentially be invalidated if an
	// administrator "steals" locks, but that should be an emergencies-only
	// situation. That _might_ be detected if the underlying storage returns an
	// error when asked to unlock a stolen lock, but storages are not required
	// to track that.)
	//
	// activeLocks and poisoned mey be accessed only while holding a suitable
	// lock on mu.
	activeLocks map[statestore.Key]bool
	poisoned    bool
	mu          sync.RWMutex
}

func newStateLocker(storage statestore.Storage) *stateLocker {
	return &stateLocker{
		storage:     storage,
		activeLocks: make(map[statestore.Key]bool),
		poisoned:    false, // becomes poisoned only if a Lock or Unlock call fails
	}
}

func (t *stateLocker) HaveAnyLock(key statekeys.Key) bool {
	anyLock, _ := t.lockStatus(key)
	return anyLock
}

func (t *stateLocker) HaveSharedLock(key statekeys.Key) bool {
	anyLock, exclusiveLock := t.lockStatus(key)
	return anyLock && !exclusiveLock
}

func (t *stateLocker) HaveExclusiveLock(key statekeys.Key) bool {
	anyLock, exclusiveLock := t.lockStatus(key)
	return anyLock && exclusiveLock
}

func (t *stateLocker) Lock(ctx context.Context, wantShared iter.Seq[statekeys.Key], wantExclusive iter.Seq[statekeys.Key]) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.storage == nil {
		return fmt.Errorf("state storage locker is now closed")
	}
	if t.poisoned {
		return fmt.Errorf("refusing to issue new state storage lock due to earlier error")
	}

	// We'll collect up the two sets of lock keys into storage-level KeySets
	// and then pass them all at once so that state storage implementations
	// can perform batch requests if their underlying storage supports that.
	var wantSharedStorage, wantExclusiveStorage statestore.KeySet
	var err error
	if wantShared != nil {
		wantSharedStorage = statestore.NewKeySet()
		for key := range wantShared {
			storageKey := key.ForStorage()
			if haveLock, _ := t.lockStatusInternal(storageKey); haveLock {
				err = errors.Join(err, fmt.Errorf("already have lock for %#v", key))
				continue
			}
			wantSharedStorage[storageKey] = struct{}{}
		}
	}
	if wantExclusive != nil {
		wantExclusiveStorage = statestore.NewKeySet()
		for key := range wantExclusive {
			storageKey := key.ForStorage()
			if haveLock, _ := t.lockStatusInternal(storageKey); haveLock {
				err = errors.Join(err, fmt.Errorf("already have lock for %#v", key))
				continue
			}
			if wantSharedStorage.Has(storageKey) {
				err = errors.Join(err, fmt.Errorf("can't request both shared and exclusive locks for %#v", key))
				continue
			}
			wantExclusiveStorage[storageKey] = struct{}{}
		}
	}
	if err != nil {
		return err
	}
	if len(wantSharedStorage) == 0 && len(wantExclusiveStorage) == 0 {
		return nil // nothing to do!
	}

	err = t.storage.Lock(ctx, wantSharedStorage, wantExclusiveStorage)
	if err != nil {
		// If Lock fails then the remote storage is in an unspecified state:
		// it may have successfully acquired a subset of the locks we requested,
		// or none at all. So we'll mark our records as poisoned to force
		// future lock requests to fail quickly so we can exit.
		t.poisoned = true
		return err // intentionally passing through underlying error directly
	}

	// If the lock request was successful then we'll remember what we locked
	// in our own data structure for future reference.
	for storageKey := range wantSharedStorage {
		t.activeLocks[storageKey] = false
	}
	for storageKey := range wantExclusiveStorage {
		t.activeLocks[storageKey] = true
	}
	return nil
}

func (t *stateLocker) Unlock(ctx context.Context, keys iter.Seq[statekeys.Key]) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.storage == nil {
		return fmt.Errorf("state storage locker is now closed")
	}
	// We intentionally don't check poisoned here because we want to try our
	// best to release as many locks as possible even if earlier lock requests
	// failed.

	var keysStorage statestore.KeySet
	var err error
	if keys != nil {
		keysStorage = statestore.NewKeySet()
		for key := range keys {
			storageKey := key.ForStorage()
			if haveLock, _ := t.lockStatusInternal(storageKey); !haveLock {
				err = errors.Join(err, fmt.Errorf("don't have any lock for %#v", key))
				continue
			}
			keysStorage[storageKey] = struct{}{}
		}
	}
	if err != nil {
		return err
	}
	if len(keysStorage) == 0 {
		return nil // nothing to do!
	}

	err = t.storage.Unlock(ctx, keysStorage)
	if err != nil {
		// If Unlock fails then the remote storage is in an unspecified state:
		// it may have successfully released a subset of the locks we requested,
		// or none at all. So we'll mark our records as poisoned to force
		// future lock requests to fail quickly so we can exit.
		t.poisoned = true
		return err // intentionally passing through underlying error directly
	}

	// If the unlock request was successful then we'll forget our records of
	// what we've just unlocked.
	for storageKey := range keysStorage {
		delete(t.activeLocks, storageKey)
	}
	return nil
}

// Close makes a best effort to release all of the active locks, and then
// makes the receiver return errors on future lock/unlock requests.
//
// The error result, if any, is the result of asking the underlying state
// storage to release all of the locks. The stateLocker is always closed on
// return, regardless of any error.
//
// This is intended to support a shutdown/cleanup sequence where we relinquish
// all remaining locks before shutting down, even if execution is halted by
// an error partway through some work.
//
// The underlying state storage is NOT closed; another part of the system is
// still responsible for ensuring that the overall state storage is properly
// closed before it's discarded.
func (t *stateLocker) Close(ctx context.Context) error {
	t.mu.Lock()
	storage := t.storage
	activeLocks := t.activeLocks
	t.storage = nil     // this object is no longer usable
	t.activeLocks = nil // to ensure we won't try to double-unlock
	t.mu.Unlock()       // we won't access the reciever anymore after this

	if len(activeLocks) == 0 {
		return nil // happy path: all locks explicitly released before we're closed
	}

	keysStorage := make(statestore.KeySet, len(activeLocks))
	for k := range activeLocks {
		keysStorage[k] = struct{}{}
	}
	return storage.Unlock(ctx, keysStorage)
}

func (t *stateLocker) lockStatus(key statekeys.Key) (anyLock, exclusiveLock bool) {
	t.mu.RLock()
	exclusiveLock, anyLock = t.lockStatusInternal(key.ForStorage())
	t.mu.RUnlock()
	return anyLock, exclusiveLock
}

// lockStatusInternal must be called only while already holding at least a
// read lock on t.mu.
func (t *stateLocker) lockStatusInternal(key statestore.Key) (anyLock, exclusiveLock bool) {
	exclusiveLock, anyLock = t.activeLocks[key]
	return anyLock, exclusiveLock
}
