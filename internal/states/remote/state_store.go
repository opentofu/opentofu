// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package remote

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"iter"
	"maps"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/opentofu/opentofu/internal/collections"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/internal/states/statestore"
	"github.com/opentofu/opentofu/version"
)

// stateStorage is an implementation of [statestore.Storage] that wraps a
// [Client], doing all of the tracking of individual objects only in local
// memory and then flattening to a single blob and single global lock for
// the underlying client to work with.
type stateStorage struct {
	client ClientLocker

	// The remaining fields are where we keep track of our transient
	// data that gets flattened into a single blob and single lock
	// request to the underlying client.
	mu        sync.RWMutex
	data      map[statestore.Key]statestore.Value
	lockCount int
	lockKey   string
}

// NewStateStorage returns a [statestore.Storage] implementation wrapping the
// given remote state client.
//
// The remote state API supports the storage and later retrieval of a single
// blob, and the acquisition of a single exclusive lock. That is considerably
// less granular than the [statestore.Storage] API and so using this
// implementation has the following limitations:
//
//   - Whenever there is an active lock on any key -- whether shared or
//     exclusive -- it is backed by an exclusive lock on the entire storage, so
//     no concurrent work is possible in another process even when there are
//     no overlapping keys.
//   - The entire dataset is read into memory when the first lock is acquired,
//     and then operations work entirely in memory until the last lock is
//     released, at which point the entire data is then written back to the
//     persistent storage as one blob.
//
// Callers can optionally call [statestore.Storage.Persist] periodically to
// write a snapshot of the latest data to persistent storage without releasing
// any locks.
func NewStateStorage(client ClientLocker) statestore.Storage {
	return &stateStorage{client: client}
}

// Close implements statestore.Storage.
func (s *stateStorage) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isActive() {
		err := s.deactivate(ctx)
		if err != nil {
			return err
		}
	}
	s.client = nil // other methods will now panic if called
	return nil
}

// Keys implements statestore.Storage.
func (s *stateStorage) Keys(ctx context.Context) iter.Seq2[statestore.Key, error] {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// If we're called after already holding at least one lock then we can
	// use our in-memory data, since we assume it won't get modified as long
	// as we're holding the lock.
	if s.isActive() {
		return func(yield func(statestore.Key, error) bool) {
			s.mu.RLock()
			defer s.mu.RUnlock()
			if !s.isActive() {
				// Our "active" state changed between us returning and
				// iteration beginning, which we'll treat as an error just
				// because it doesn't seem important to support it.
				yield(statestore.NoKey, fmt.Errorf("storage lock status changed before consuming key sequence"))
				return
			}
			for k := range s.data {
				if ok := yield(k, nil); !ok {
					return
				}
			}
		}
	}

	// If we're called before any locks are acquired then we fetch directly
	// from the underlying source and return the result because we have no
	// guarantee that the underlying data won't change before the first lock
	// is acquired anyway.
	data, err := s.fetchData(ctx)
	if err != nil {
		return func(yield func(statestore.Key, error) bool) {
			yield(statestore.NoKey, err)
		}
	}
	return func(yield func(statestore.Key, error) bool) {
		for k := range data {
			if ok := yield(k, nil); !ok {
				return
			}
		}
	}
}

// Lock implements statestore.Storage.
func (s *stateStorage) Lock(ctx context.Context, shared collections.Set[statestore.Key], exclusive collections.Set[statestore.Key]) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lockCount == 0 {
		// Acquiring the first lock causes us to acquire the client's global
		// lock and then fetch the current data snapshot from it.
		err := s.activate(ctx)
		if err != nil {
			return err
		}
	}
	s.lockCount += len(shared) + len(exclusive)
	return nil
}

// Persist implements statstore.Storage.
func (s *stateStorage) Persist(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isActive() {
		// Nothing to do if we're not actually active right now.
		return nil
	}

	// For this implementation, "Persist" means to write a snapshot of the
	// current data to the remote storage so that e.g. a crash of our process
	// won't cause loss of any in-memory-only data.
	return s.persistData(ctx)
}

// Read implements statestore.Storage.
func (s *stateStorage) Read(ctx context.Context, keys collections.Set[statestore.Key]) (map[statestore.Key]statestore.Value, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.isActive() {
		return nil, fmt.Errorf("read while not holding lock")
	}
	if len(keys) == 0 {
		return nil, nil
	}
	ret := make(map[statestore.Key]statestore.Value, len(keys))
	for key := range keys {
		ret[key] = s.data[key]
	}
	return ret, nil
}

// Unlock implements statestore.Storage.
func (s *stateStorage) Unlock(ctx context.Context, keys collections.Set[statestore.Key]) error {
	s.lockCount -= len(keys)
	if s.lockCount < 0 { // Caller seems to have a bug
		s.lockCount = 0
		return fmt.Errorf("released more locks than have been acquired")
	}
	if s.lockCount == 0 {
		err := s.deactivate(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// Write implements statestore.Storage.
func (s *stateStorage) Write(ctx context.Context, data map[statestore.Key]statestore.Value) error {
	s.mu.Lock()
	defer s.mu.Lock()

	if !s.isActive() {
		return fmt.Errorf("write while not holding lock")
	}
	for key, value := range data {
		s.data[key] = value
	}
	return nil
}

func (s *stateStorage) isActive() bool {
	return s.data != nil
}

// activate prepares the storage for reading and writing. This should be called
// only by [stateStorage.Lock], while holding an exclusive lock on our mutex.
func (s *stateStorage) activate(ctx context.Context) (err error) {
	lockKey, err := s.client.Lock(&statemgr.LockInfo{
		Version: version.Version,
		Created: time.Now(),
	})
	if err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	s.lockKey = lockKey
	defer func() {
		// If we're returning an error after this then we'll try
		// to release the lock before we return, because otherwise it'll
		// get stuck on.
		if err != nil {
			s.lockKey = ""
			_ = s.client.Unlock(lockKey) // Intentionally ignored; this is best-effort.
		}
	}()

	data, err := s.fetchData(ctx)
	if err != nil {
		return err
	}
	s.data = data
	return nil
}

// deactivate persists our in-memory data to the underlying storage, and
// releases the remote lock. This should be called only while holding an
// exclusive lock on our mutex.
func (s *stateStorage) deactivate(ctx context.Context) error {
	err := s.persistData(ctx)
	if err != nil {
		return err
	}

	err = s.client.Unlock(s.lockKey)
	if err != nil {
		return fmt.Errorf("releasing lock: %w", err)
	}

	s.data = nil
	s.lockKey = ""
	return nil
}

func (s *stateStorage) fetchData(_ context.Context) (map[statestore.Key]statestore.Value, error) {
	payload, err := s.client.Get()
	if err != nil {
		return nil, fmt.Errorf("fetching state data: %w", err)
	}
	data, err := s.unmarshalData(payload.Data)
	if err != nil {
		return nil, fmt.Errorf("invalid state data: %w", err)
	}
	return data, nil
}

func (s *stateStorage) persistData(_ context.Context) error {
	data := s.marshalData(s.data)
	err := s.client.Put(data)
	if err != nil {
		return fmt.Errorf("storing state data: %w", err)
	}
	return nil
}

func (s *stateStorage) unmarshalData(raw []byte) (map[statestore.Key]statestore.Value, error) {
	if len(raw) < stateStorageHeaderLen || !bytes.HasPrefix(raw, []byte(stateStorageMagic)) {
		return nil, fmt.Errorf("missing or incorrect header")
	}
	wantItemCountRaw := binary.BigEndian.Uint64(raw[stateStorageHeaderLen-8 : stateStorageHeaderLen])
	if wantItemCountRaw > math.MaxInt {
		// It shouldn't actually be possible to get here anyway because
		// "raw" cannot be bigger than math.MaxInt and so we can't possibly
		// have wantItemCountRaw items in the buffer in practice, so this
		// check is just here to make sure that the below conversion to int
		// cannot possibly silently overflow, but should be unreachable.
		return nil, fmt.Errorf("too many state items")
	}
	wantItemCount := int(wantItemCountRaw)
	ret := make(map[statestore.Key]statestore.Value, wantItemCount)
	// We're done with the header and will now focus only on the content.
	r := bytes.NewReader(raw[stateStorageHeaderLen:])
	for len(raw) > 0 {
		keyNameLenRaw, err := binary.ReadUvarint(r)
		if err != nil {
			return nil, fmt.Errorf("reading key name length: %w", err)
		}
		if keyNameLenRaw > math.MaxInt {
			return nil, fmt.Errorf("key name too long")
		}
		keyNameLen := int(keyNameLenRaw)
		keyNameBuf := make([]byte, keyNameLen)
		n, err := r.Read(keyNameBuf)
		if err != nil {
			return nil, fmt.Errorf("reading key name: %w", err)
		}
		if n != keyNameLen {
			return nil, fmt.Errorf("end of data during key name")
		}
		key, err := statestore.ParseKey(string(keyNameBuf))
		if err != nil {
			return nil, fmt.Errorf("invalid state key %q", keyNameBuf)
		}

		valueLenRaw, err := binary.ReadUvarint(r)
		if err != nil {
			return nil, fmt.Errorf("reading value length: %w", err)
		}
		if valueLenRaw == 0 {
			return nil, fmt.Errorf("zero-length value for %q", key.Name()) // not allowed
		}
		if valueLenRaw > math.MaxInt {
			return nil, fmt.Errorf("value for %q too long", key.Name())
		}
		valueLen := int(valueLenRaw)
		valueBuf := make([]byte, valueLen)
		n, err = r.Read(valueBuf)
		if err != nil {
			return nil, fmt.Errorf("reading value for %q: %w", key.Name(), err)
		}
		if n != valueLen {
			return nil, fmt.Errorf("end of data during %q value", key.Name())
		}
		value := statestore.Value(valueBuf)

		ret[key] = value
	}
	if len(ret) != wantItemCount {
		return nil, fmt.Errorf("expected %d items, but found %d", wantItemCount, len(ret))
	}
	return ret, nil
}

func (s *stateStorage) marshalData(data map[statestore.Key]statestore.Value) []byte {
	keys := slices.Collect(maps.Keys(data))
	slices.SortFunc(keys, func(a, b statestore.Key) int {
		switch {
		case a == b:
			return 0
		case a.Name() < b.Name():
			return -1
		default:
			return 1
		}
	})

	var buf bytes.Buffer
	buf.WriteString(stateStorageMagic)
	buf.Write(binary.BigEndian.AppendUint64(buf.AvailableBuffer(), uint64(len(keys))))
	for _, key := range keys {
		value := data[key]
		keyName := key.Name()
		buf.Write(binary.AppendUvarint(buf.AvailableBuffer(), uint64(len(keyName))))
		buf.WriteString(keyName)
		buf.Write(binary.AppendUvarint(buf.AvailableBuffer(), uint64(len(value))))
		buf.Write(value)
	}
	return buf.Bytes()
}

const stateStorageMagic = "TOFU\x00\x00\x00\x01"
const stateStorageHeaderLen = len(stateStorageMagic) + 8
