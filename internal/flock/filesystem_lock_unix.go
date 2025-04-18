// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !windows
// +build !windows

package flock

import (
	"io"
	"os"
	"syscall"
)

// use fcntl POSIX locks for the most consistent behavior across platforms, and
// hopefully some compatibility over NFS and CIFS.
func Lock(f *os.File) error {
	flock := &syscall.Flock_t{
		Type:   syscall.F_RDLCK | syscall.F_WRLCK,
		Whence: int16(io.SeekStart),
		Start:  0,
		Len:    0,
	}

	return syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, flock)
}

func Unlock(f *os.File) error {
	flock := &syscall.Flock_t{
		Type:   syscall.F_UNLCK,
		Whence: int16(io.SeekStart),
		Start:  0,
		Len:    0,
	}

	return syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, flock)
}
