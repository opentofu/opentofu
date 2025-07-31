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

// Similar to Lock, but acquires the lock asynchronously and returns the result (nil or error) in a channel.
// A cancel function is provided to interrupt the lock waiting process
//
// Given the cancel function fires a signal that cancels the lock, this is not safe to use in multiple
// go-routines in parallel.  If the ability to make this call in parallel is desired, the details of the
// cancel implementation could be tweaked.
func LockBlocking(f *os.File) (chan error, func()) {
	flock := &syscall.Flock_t{
		Type:   syscall.F_RDLCK | syscall.F_WRLCK,
		Whence: int16(io.SeekStart),
		Start:  0,
		Len:    0,
	}

	c := make(chan error)

	go func() {
		c <- syscall.FcntlFlock(f.Fd(), syscall.F_SETLKW, flock)
		close(c)
	}()

	return c, func() {
		/* From man fcntl
		   If a signal is caught while waiting, then the call is interrupted and
		   (after the signal handler has returned) returns immediately
		   (with return value -1 and errno set to EINTR

		   We choose SIGUSR1 instead of SIGINT to allow tofu to continue with it's normal error handling routine
		*/

		syscall.Kill(syscall.Getpid(), syscall.SIGUSR1)
	}
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
