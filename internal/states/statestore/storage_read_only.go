// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statestore

import (
	"context"
	"fmt"
	"iter"

	"github.com/opentofu/opentofu/internal/collections"
)

// AsReadOnly returns the given storage wrapped in an adapter that forces it
// to return errors if a caller attempts to perform any operation related
// to modifying the stored state data.
//
// Acquiring and releasing shared locks is still allowed, but acquiring
// exclusive locks is forbidden because it's only useful to do that if
// you intend to modify something.
//
// We use this as a guard against bugs where code is inappropriately shared
// between plan and apply operations, to increase the likelihood of that
// mistake causing an error instead of doing something unexpected and
// potentially harmful. However, it's not 100% robust to all mistakes and so
// this should be treated only as an internal guardrail, and not relied on for
// handling intentional differences between plan and apply.
func AsReadOnly(underlying Storage) Storage {
	if _, alreadyWrapped := underlying.(readOnlyStorage); alreadyWrapped {
		return underlying // avoid direct double-wrapping
	}
	return readOnlyStorage{underlying}
}

type readOnlyStorage struct {
	underlying Storage
}

// Close implements Storage.
func (r readOnlyStorage) Close(ctx context.Context) error {
	return r.underlying.Close(ctx)
}

// Keys implements Storage.
func (r readOnlyStorage) Keys(ctx context.Context) iter.Seq2[Key, error] {
	return r.underlying.Keys(ctx)
}

// Lock implements Storage.
func (r readOnlyStorage) Lock(ctx context.Context, shared collections.Set[Key], exclusive collections.Set[Key]) error {
	if len(exclusive) != 0 {
		return fmt.Errorf("attempted to acquire exclusive locks on read-only state storage")
	}
	return r.Lock(ctx, shared, nil)
}

// Persist implements Storage, although it does nothing because a read-only
// storage should have no change to persist.
func (r readOnlyStorage) Persist(ctx context.Context) error {
	return nil
}

// Read implements Storage.
func (r readOnlyStorage) Read(ctx context.Context, keys collections.Set[Key]) (map[Key]Value, error) {
	return r.underlying.Read(ctx, keys)
}

// Unlock implements Storage.
func (r readOnlyStorage) Unlock(ctx context.Context, keys collections.Set[Key]) error {
	return r.underlying.Unlock(ctx, keys)
}

// Write implements Storage.
func (r readOnlyStorage) Write(context.Context, map[Key]Value) error {
	return fmt.Errorf("attempted to write to read-only state storage")
}
