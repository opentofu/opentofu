// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !windows

package flock

import (
	"context"
	"fmt"
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

// LockBlocking is like Lock except that if the lock is currently contended
// then it blocks until it becomes available.
//
// If the given context is cancelled then it returns early with the cancellation
// error.
func LockBlocking(ctx context.Context, f *os.File) error {
	flock := &syscall.Flock_t{
		Type:   syscall.F_RDLCK | syscall.F_WRLCK,
		Whence: int16(io.SeekStart),
		Start:  0,
		Len:    0,
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	c := make(chan error)
	go func() {
		for {
			err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLKW, flock)
			if err == syscall.EINTR {
				// We'll get here if our process gets any signal at all, but
				// not all signals represent cancellation.
				if ctxErr := ctx.Err(); ctxErr != nil {
					err = ctxErr // return the cancellation error instead of generic EINTR
				} else {
					continue // not cancelled yet
				}
			}
			c <- err
			close(c)
			return
		}
	}()

	for {
		select {
		case err := <-c:
			return err
		case <-ctx.Done():
			// We will get here if the cancellation is caused by anything other
			// than a Unix signal, in which case we'll send a signal ourselves
			// to force the waiting goroutine to exit.
			// We use SIGUSR1 here on the assumption that nothing else in
			// OpenTofu uses it. We're sending this to the current pid
			// explicitly because we might have other processes, such as
			// plugins, also running in our process group (which is what we'd
			// signal if using pid 0 here).
			err := syscall.Kill(syscall.Getpid(), syscall.SIGUSR1)
			if err != nil {
				// This should not fail, but if it does then we'd otherwise
				// get hung up here and so we'll return an error and accept
				// that our background goroutine is going to just hang around
				// until another signal shows up or the program exits.
				return fmt.Errorf("failed canceling lock acquisition: %w", err)
			}
		}
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
