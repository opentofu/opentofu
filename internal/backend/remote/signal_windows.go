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
	"unsafe"

	"golang.org/x/sys/windows"
)

// sendInterruptSignal sends an Ctrl+Break event to the given process ID.
func sendInterruptSignal(pid int) error {
	err := windows.GenerateConsoleCtrlEvent(syscall.CTRL_BREAK_EVENT, uint32(pid))
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

func listenForConsoleCtrlHandler(resultCh chan struct{}) {
	cb := func(dwCtrlType uint32) uintptr {
		switch dwCtrlType {
		case syscall.CTRL_C_EVENT:
			resultCh <- struct{}{}
			return 1
		default:
			return 0 // Let other handlers or the default handler process the event
		}
	}
	ret, _, err := setConsoleCtrlHandler.Call(
		syscall.NewCallback(cb), // Pointer to our handler function
		uintptr(1),              // Add the handler (TRUE)
		uintptr(unsafe.Pointer(&resultCh)),
	)
	if ret == 0 || err != nil {
		log.Printf("[ERROR] error setting console ctrl handler: %v", err)
	}
}

func stopCtrlHandler(resultCh chan struct{}) {
	cb := func(dwCtrlType uint32) uintptr {
		switch dwCtrlType {
		case syscall.CTRL_C_EVENT:
			resultCh <- struct{}{}
			return 1
		default:
			return 0 // Let other handlers or the default handler process the event
		}
	}
	ret, _, err := setConsoleCtrlHandler.Call(
		syscall.NewCallback(cb), // Pointer to our handler function
		uintptr(0),              // Remove the handler (FALSE)
		uintptr(unsafe.Pointer(&resultCh)),
	)
	if ret == 0 || err != nil {
		log.Printf("[ERROR] error unsetting console ctrl handler: %v", err)
	}
}
