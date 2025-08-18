// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package statestore defines our internal abstraction for mutable state
// storage, along with vocabulary types and utilities needed by implementations
// of this abstraction.
//
// State storage is conceptually a key/value store where keys are strings,
// values are opaque blobs, and each key can be independently locked in either
// a shared or an exclusive fashion. A fully-fledged state storage
// implementation can:
//
//   - Enumerate all of the keys currently stored.
//   - Read the blobs associated with a given set of keys.
//   - Acquire shared or exclusive locks for a given set of keys.
//   - Write or delete blobs for a given set of keys for which exclusive locks
//     are already held.
//   - Release previously-acquired locks on a given set of keys.
//   - Force any existing lock on each of a set of keys to be released, as an
//     administrative workaround for state storage left in an inconsistent
//     state due to catastrophic failure.
//
// However, implementations are permitted to make certain compromises that
// reduce opportunities for concurrency and failure-resilience but do not
// sacrifice correctness when the system is behaving as designed:
//
//   - Remotely track only a single shared/exclusive lock (aka read/write lock)
//     for the entire storage rather than on a per-key basis, and track the
//     per-key locks only locally within the current process.
//   - Remotely track only exclusive locks, tracking the shared vs. exclusive
//     distinction only locally within the current process.
//   - Track the entire state as a single blob, with the key/value granularity
//     managed only locally within the current process and the whole blob
//     flushed to persistent storage together. (This compromise requires that
//     locks are also tracked at the same whole-state granularity or that the
//     implementations are able to cooperate to resolve write conflicts, so that
//     multiple processes to not race to update the same blob and erase
//     each other's changes.)
//
// Callers of [Storage] treat the locking mechanism as advisory, but
// implementations are allowed to use mandatory locks for additional assurance
// if their underlying storage supports them.
//
// Refer to [Storage] for more details on the requirements for implementors.
//
// State storage implementations are not permitted to assume any special meaning
// of the keys and values that their caller requests to store, whether
// client-side or server-side. The object types and their storage
// representations are subject to change in future OpenTofu releases and
// are considered an implementation detail. External software that needs to
// interact with stored OpenTofu state must do so via other documented
// integration points, and must not access the underlying state storage
// directly.
package statestore
