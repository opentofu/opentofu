// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows
// +build windows

package remote

import (
	"log"
	"os"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

// sendInterruptSignal sends an Ctrl+Break event to the given process ID.
func sendInterruptSignal(pid int) error {
	err := windows.GenerateConsoleCtrlEvent(syscall.CTRL_C_EVENT, uint32(pid))
	if err != nil {
		return err
	}
	return nil
}

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	setConsoleCtrlHandler = kernel32.NewProc("SetConsoleCtrlHandler")
)

// ConsoleCtrlHandler is our custom handler routine

var ignoreSignals = []os.Signal{os.Interrupt}
var forwardSignals = []os.Signal{}

func handleSignals(t *testing.T, resultCh chan struct{}) (func(t *testing.T), error) {
	cb := syscall.NewCallback(func(dwCtrlType uint32) uintptr {
		t.Logf("console ctrl handler called with type: %d", dwCtrlType)
		switch dwCtrlType {
		case syscall.CTRL_C_EVENT:
			resultCh <- struct{}{}
			return 1
		case syscall.CTRL_BREAK_EVENT:
			resultCh <- struct{}{}
			return 1
		default:
			return 0 // Let other handlers or the default handler process the event
		}
	})

	// pcb := syscall.NewCallback(cb)
	t.Logf("setting console ctrl handler")
	ret, _, err := setConsoleCtrlHandler.Call(
		cb,         // Pointer to our handler function
		uintptr(1), // Add the handler (TRUE)
	)

	if ret == 0 && err != nil {
		log.Printf("[ERROR] error setting console ctrl handler: %v", err)
	}

	unregisterFn := func(t *testing.T) {
		t.Logf("unregistering console ctrl handler")
		ret, _, err := setConsoleCtrlHandler.Call(
			cb,         // Remove the handler (FALSE)
			uintptr(0), // Remove the handler (FALSE)
		)
		if ret == 0 && err != nil {
			t.Logf("error unsetting console ctrl handler: %v", err)
		}
	}

	return unregisterFn, nil
}
