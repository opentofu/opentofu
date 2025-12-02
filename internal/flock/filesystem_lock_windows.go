// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows

package flock

import (
	"context"
	"errors"
	"log"
	"math"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procCreateEventW = modkernel32.NewProc("CreateEventW")
)

const (
	// dwFlags defined for LockFileEx
	// https://msdn.microsoft.com/en-us/library/windows/desktop/aa365203(v=vs.85).aspx
	_LOCKFILE_FAIL_IMMEDIATELY = 1
	_LOCKFILE_EXCLUSIVE_LOCK   = 2
	// https://learn.microsoft.com/en-us/windows/win32/debug/system-error-codes--0-499-
	ERROR_LOCK_VIOLATION = 33
)

// This still allows the file handle to be opened by another process for competing locks on the same file.
func Lock(f *os.File) error {
	// even though we're failing immediately, an overlapped event structure is
	// required
	ol, err := newOverlapped()
	if err != nil {
		return err
	}
	defer func() {
		err := syscall.CloseHandle(ol.HEvent)
		if err != nil {
			log.Printf("[ERROR] failed to close file locking event handle: %v", err)
		}
	}()

	return lockFileEx(
		syscall.Handle(f.Fd()),
		_LOCKFILE_EXCLUSIVE_LOCK|_LOCKFILE_FAIL_IMMEDIATELY,
		0,              // reserved
		0,              // bytes low
		math.MaxUint32, // bytes high
		ol,
	)
}

// This is a poor implementation of blocking locks, but it a somewhat function patch for the moment.
// This should eventually be tweaked to use native windows locking.
// See https://github.com/opentofu/opentofu/issues/3089 for more details.
func LockBlocking(ctx context.Context, f *os.File) error {
	resultChan := make(chan error)

	go func() {
		for {
			err := Lock(f)
			if err == nil {
				// Lock succeeded
				resultChan <- nil
				return
			}

			select {
			case <-ctx.Done():
				// Lock cancelled, so return cancellation error
				resultChan <- ctx.Err()
				return
			default:
				// LockFileEx returns this error when the lock is contended.
				var errno syscall.Errno
				ok := errors.As(err, &errno)
				if ok && errno == ERROR_LOCK_VIOLATION {
					// Chill for a bit before trying again
					time.Sleep(100 * time.Millisecond)
					continue
				}
				// All other errors are fatal.
				resultChan <- err
			}
		}
	}()

	return <-resultChan
}

func Unlock(*os.File) error {
	// the lock is released when Close() is called
	return nil
}

func lockFileEx(h syscall.Handle, flags, reserved, locklow, lockhigh uint32, ol *syscall.Overlapped) (err error) {
	r1, _, e1 := syscall.SyscallN(
		procLockFileEx.Addr(),
		uintptr(h),
		uintptr(flags),
		uintptr(reserved),
		uintptr(locklow),
		uintptr(lockhigh),
		uintptr(unsafe.Pointer(ol)),
	)
	if r1 == 0 {
		if e1 != 0 {
			err = error(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

// newOverlapped creates a structure used to track asynchronous
// I/O requests that have been issued.
func newOverlapped() (*syscall.Overlapped, error) {
	event, err := createEvent(nil, true, false, nil)
	if err != nil {
		return nil, err
	}
	return &syscall.Overlapped{HEvent: event}, nil
}

func createEvent(sa *syscall.SecurityAttributes, manualReset bool, initialState bool, name *uint16) (handle syscall.Handle, err error) {
	var _p0 uint32
	if manualReset {
		_p0 = 1
	}
	var _p1 uint32
	if initialState {
		_p1 = 1
	}

	r0, _, e1 := syscall.SyscallN(
		procCreateEventW.Addr(),
		uintptr(unsafe.Pointer(sa)),
		uintptr(_p0),
		uintptr(_p1),
		uintptr(unsafe.Pointer(name)),
		0,
		0,
	)
	handle = syscall.Handle(r0)
	if handle == syscall.InvalidHandle {
		if e1 != 0 {
			err = error(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}
