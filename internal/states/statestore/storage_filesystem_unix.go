// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !windows
// +build !windows

package statestore

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func (f *FilesystemStorage) lockFileShared(target *os.File) error {
	flock := &unix.Flock_t{
		Type:   unix.F_RDLCK,
		Whence: int16(io.SeekStart),
		Start:  0,
		Len:    0,
	}
	return unix.FcntlFlock(target.Fd(), unix.F_SETLK, flock)
}

func (f *FilesystemStorage) lockFileExclusive(target *os.File) error {
	flock := &unix.Flock_t{
		Type:   unix.F_WRLCK,
		Whence: int16(io.SeekStart),
		Start:  0,
		Len:    0,
	}
	return unix.FcntlFlock(target.Fd(), unix.F_SETLK, flock)
}

func (f *FilesystemStorage) unlockFile(target *os.File) error {
	flock := &unix.Flock_t{
		Type:   unix.F_UNLCK,
		Whence: int16(io.SeekStart),
		Start:  0,
		Len:    0,
	}
	return unix.FcntlFlock(target.Fd(), unix.F_SETLK, flock)
}

// isContendedFilesystemLockError returns true if the given error is one that
// [FilesystemStorage.lockFileShared] or [FilesystemStorage.lockFileExclusive]
// would return to indicate that the requested lock is contended.
func isContendedFilesystemLockError(err error) bool {
	errno, ok := err.(unix.Errno)
	if !ok {
		return false
	}
	switch errno {
	case unix.EAGAIN, unix.EACCES, unix.EINTR:
		// The exact error code used for "contended" varies between operating
		// systems, so we'll allow all three above.
		return true
	default:
		return false
	}
}
