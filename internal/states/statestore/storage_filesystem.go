// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statestore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/opentofu/opentofu/internal/collections"
)

// FilesystemStorage is an implementation of [Storage] that uses files in
// a directory accessible using the current operating system's filesystem
// abstraction.
//
// Locking for this implementation relies on "flock" for Unix-style systems
// and `LockFileEx` on Windows systems, and so this implementation is safe
// to use only in filesystems where those mechanisms work reliably. For
// example, this implementation is not safe to use on some network filesystems.
type FilesystemStorage struct {
	// root is the directory where we store our objects and
	root *os.Root

	// The remaining fields track our own locking state, so we can support
	// nested locking and retain the file descriptors that our OS-level
	// locks are associated with.
	mu    sync.RWMutex
	locks map[Key]*filesystemStorageLock // initialized on first lock request
}

var _ Storage = (*FilesystemStorage)(nil)

// We create all files with the following suffix on their names so that we
// can tolerate most situations where other software on a computer might
// rudely add files into a directory, such as a file manager saving a
// cache of metadata it's extracted from other files in the directory.
const filesystemStorageFilenameSuffix = ".tofustate"

// OpenFilesystemStorage creates a new [FilesystemStorage] object that
// uses the given directory path for storage.
//
// Concurrent processes can safely work in the same directory as long as they
// are all using this implementation. Any other changes to the directory
// (including any content placed in that directory prior to its first use
// with this function) causes unspecified behavior.
func OpenFilesystemStorage(dir string) (*FilesystemStorage, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &FilesystemStorage{root: root}, nil
}

// Close implements Storage.
func (f *FilesystemStorage) Close(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var err error

	// We'll try to release all of our active locked first.
	locked := collections.NewSet[Key]()
	for key := range f.locks {
		locked[key] = struct{}{}
	}
	err = errors.Join(err, f.unlockInner(ctx, locked))

	// Now we'll close our directory handle, which will make future
	// actions on this object fail with an error.
	err = errors.Join(err, f.root.Close())
	return err
}

// Keys implements Storage.
func (f *FilesystemStorage) Keys(ctx context.Context) iter.Seq2[Key, error] {
	return func(yield func(Key, error) bool) {
		// NOTE: The result from os.Root.FS is guaranteed by its documentation
		// to implement fs.ReadDirFS.
		entries, err := f.root.FS().(fs.ReadDirFS).ReadDir(".")
		if err != nil {
			yield(Key{}, err)
			return
		}
		for _, entry := range entries {
			if !entry.Type().IsRegular() {
				continue // We only care about regular files
			}
			filename := entry.Name()
			rawKey := strings.TrimSuffix(filename, filesystemStorageFilenameSuffix)
			if len(rawKey) == len(filename) {
				continue // filename did not end with the expected suffix
			}
			key, err := ParseKey(rawKey)
			if err != nil {
				continue // ignore anything that isn't a valid key
			}
			if err := ctx.Err(); err != nil { // respond to context cancellation in case we're on a slow filesystem
				yield(key, err)
				return
			}
			info, err := entry.Info()
			if err != nil || info.Size() == 0 {
				continue // ignore empty files and files that have vanished since we listed the directory
			}
			if !yield(key, nil) {
				return
			}
		}
	}
}

// Lock implements Storage.
func (f *FilesystemStorage) Lock(ctx context.Context, shared collections.Set[Key], exclusive collections.Set[Key]) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.locks == nil {
		f.locks = make(map[Key]*filesystemStorageLock)
	}

	// We aren't required to acquire the locks in any particular order as
	// long as we have acquired them all by the time we return successfully,
	// so we'll arbitrarily deal with the exclusive locks just because
	// they are more likely to be contended.
	err := f.acquireLocks(ctx, exclusive, true, f.lockFileExclusive)
	if err != nil {
		return err
	}
	return f.acquireLocks(ctx, shared, false, f.lockFileShared)
}

// acquireLocks must only be called by [FilesystemStorage.Lock], while holding
// a lock on f.mu.
func (f *FilesystemStorage) acquireLocks(ctx context.Context, want collections.Set[Key], exclusive bool, lockFunc func(target *os.File) error) error {
	for key := range want {
		_, ok := f.locks[key]
		if ok {
			// This object already has a lock on this key, so our caller
			// is buggy and not properly tracking what it has locked.
			return fmt.Errorf("lock conflict for %q", key.Name())
		}

		filename := f.filename(key)

		// O_CREATE without O_EXCL means that we'll create the file if it
		// doesn't already exist but just open it normally if it does.
		// If we create the file then it will initially be empty, which
		// is okey because [FilesystemStorage.Keys] ignores empty files.
		file, err := f.root.OpenFile(filename, os.O_CREATE|os.O_RDONLY, os.ModePerm)
		if err != nil {
			return err
		}
		err = f.waitForFileLock(ctx, file, lockFunc)
		if err != nil {
			return err
		}
		f.locks[key] = &filesystemStorageLock{
			file:      file,
			exclusive: exclusive,
		}
	}
	return nil
}

func (f *FilesystemStorage) waitForFileLock(ctx context.Context, target *os.File, lockFunc func(target *os.File) error) error {
	for {
		err := lockFunc(target)
		if err == nil || !isContendedFilesystemLockError(err) {
			return err
		}

		// We'll wait a little before we poll again, unless the given context
		// gets cancelled.
		timer := time.NewTimer(time.Second)
		select {
		case <-timer.C:
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Persist implements Storage.
func (f *FilesystemStorage) Persist(_ context.Context) error {
	// Nothing special to do in this implementation, because Write persists
	// immediately after each change.
	return nil
}

// Read implements Storage.
func (f *FilesystemStorage) Read(ctx context.Context, want collections.Set[Key]) (map[Key]Value, error) {
	if len(want) == 0 {
		return nil, nil
	}
	f.mu.RLock()
	defer f.mu.RUnlock()

	ret := make(map[Key]Value, len(want))
	for key := range want {
		lock := f.locks[key]
		if lock == nil {
			// We don't have an active lock for this file, so the caller is buggy.
			return nil, fmt.Errorf("reading %q while not holding lock", key.Name())
		}

		value, err := readValueFromFile(lock.file)
		if err != nil {
			return nil, fmt.Errorf("reading %q: %w", key.Name(), err)
		}
		ret[key] = value
	}
	return ret, nil
}

// Unlock implements Storage.
func (f *FilesystemStorage) Unlock(ctx context.Context, keys collections.Set[Key]) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.unlockInner(ctx, keys)
}

func (f *FilesystemStorage) unlockInner(_ context.Context, keys collections.Set[Key]) error {
	for key := range keys {
		lock := f.locks[key]
		if lock == nil {
			return fmt.Errorf("unlocking %q while not holding lock", key.Name())
		}

		// Before we get too far here we'll try to make sure that any changes
		// we've made are committed to disk, rather than just in cache.
		err := lock.file.Sync()
		if err != nil {
			return fmt.Errorf("sync %q before unlock: %w", key.Name(), err)
		}

		err = f.unlockFile(lock.file)
		if err != nil {
			return fmt.Errorf("unlocking %q: %w", key.Name(), err)
		}
		delete(f.locks, key)
		err = lock.file.Close()
		if err != nil {
			// Weird to get here since we synced already above, but okay...
			return fmt.Errorf("closing file for %q: %w", key.Name(), err)
		}
	}
	return nil
}

// Write implements Storage.
func (f *FilesystemStorage) Write(ctx context.Context, new map[Key]Value) error {
	if len(new) == 0 {
		return nil
	}
	// We use exclusive locking for writes to ensure that we can't have
	// two goroutines in our process trying to write to the same file
	// concurrently.
	f.mu.Lock()
	defer f.mu.Unlock()

	for key, value := range new {
		lock := f.locks[key]
		if lock == nil || !lock.exclusive {
			// We don't have an active lock for this file, so the caller is buggy.
			return fmt.Errorf("writing %q while not holding exclusive lock", key.Name())
		}

		err := writeValueToFile(value, lock.file)
		if err != nil {
			return fmt.Errorf("writing %q: %w", key.Name(), err)
		}
		err = lock.file.Sync()
		if err != nil {
			return fmt.Errorf("persisting %q: %w", key.Name(), err)
		}
	}
	return nil
}

func (f *FilesystemStorage) filename(key Key) string {
	return key.Name() + filesystemStorageFilenameSuffix
}

type filesystemStorageLock struct {
	file      *os.File
	exclusive bool
}

// readValueFromFile is similar to [io.ReadAll] but is safe for multiple
// concurrent calls on the same file because it does not use or change the
// file position as tracked by the kernel.
//
// It is NOT safe to use this function concurrently with any changes to the
// given file.
func readValueFromFile(f *os.File) (Value, error) {
	// To allow for multiple concurrent readers of the same file, we maintain
	// our own local file cursor here instead of relying on the in-kernel
	// position associated with the file descriptor.
	pos := int64(0)
	var buf bytes.Buffer
	for {
		buf.Grow(64) // arbitrary minimum buffer space to read into
		into := buf.AvailableBuffer()
		n, err := f.ReadAt(into, pos)
		if err != nil && err != io.EOF {
			return nil, err
		}
		buf.Write(into[:n])
		pos += int64(n)
		if err == io.EOF || n == 0 {
			break
		}
	}
	if buf.Len() == 0 {
		return NoValue, nil
	}
	return Value(buf.Bytes()), nil
}

// writeValueToFile replaces the content of the given file with the
// given value.
//
// It is not safe to call this function concurrently with the same file
// or concurrently with any other changes to the given file.
func writeValueToFile(value Value, f *os.File) error {
	// Although we require exclusive access, for consistency with
	// [readValueFromFile] we again avoid using the kernel's own file
	// position since we know we always want to write at the very start.
	pos := int64(0)
	remain := []byte(value)
	for len(remain) > 0 {
		n, err := f.WriteAt(remain, pos)
		if err != nil {
			if pos > 0 {
				// We've probably now corrupted the file, so we'll make
				// a best effort to leave it empty rather than corrupt, but
				// this won't necessarily succeed.
				_ = f.Truncate(0)
			}
			return err
		}
		pos += int64(n)
		remain = remain[n:]
	}
	// If we got here then we've written the new data, but if the old
	// data was longer then we still have some straggling bytes after
	// so we'll get rid of those now.
	return f.Truncate(int64(len(value)))
}
