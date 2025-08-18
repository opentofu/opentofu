// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows
// +build windows

package statestore

import (
	"math"
	"os"

	"golang.org/x/sys/windows"
)

func (f *FilesystemStorage) lockFileShared(target *os.File) error {
	ol, err := newOverlapped()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(ol.HEvent)

	return windows.LockFileEx(
		windows.Handle(target.Fd()),
		windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,              // reserved
		0,              // bytes low
		math.MaxUint32, // bytes high
		ol,
	)
}

func (f *FilesystemStorage) lockFileExclusive(target *os.File) error {
	ol, err := newOverlapped()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(ol.HEvent)

	return windows.LockFileEx(
		windows.Handle(target.Fd()),
		windows.LOCKFILE_FAIL_IMMEDIATELY|windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,              // reserved
		0,              // bytes low
		math.MaxUint32, // bytes high
		ol,
	)
}

func (f *FilesystemStorage) unlockFile(target *os.File) error {
	ol, err := newOverlapped()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(ol.HEvent)

	return windows.UnlockFileEx(
		windows.Handle(target.Fd()),
		0,              // reserved
		0,              // bytes low
		math.MaxUint32, // bytes high
		ol,
	)
}

// isContendedFilesystemLockError returns true if the given error is one that
// [FilesystemStorage.lockFileShared] or [FilesystemStorage.lockFileExclusive]
// would return to indicate that the requested lock is contended.
func isContendedFilesystemLockError(err error) bool {
	return err == windows.ERROR_IO_PENDING
}

// newOverlapped creates a structure used to track asynchronous
// I/O requests that have been issued.
func newOverlapped() (*windows.Overlapped, error) {
	handle, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, err
	}
	return &windows.Overlapped{HEvent: handle}, nil
}
