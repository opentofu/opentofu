// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows
// +build windows

package main

import (
	"log"
	"os"
	"syscall"
	"unsafe"
)

// Define the handler function signature for SetConsoleCtrlHandler
type HandlerRoutine func(dwCtrlType uint32) uintptr

// SetConsoleCtrlHandler function from kernel32.dll
var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	setConsoleCtrlHandler = kernel32.NewProc("SetConsoleCtrlHandler")
)

// ConsoleCtrlHandler is our custom handler routine
func ConsoleCtrlHandler(dwCtrlType uint32, resultCh chan struct{}) uintptr {
	switch dwCtrlType {
	case syscall.CTRL_C_EVENT:
		resultCh <- struct{}{}
		return 1
	default:
		return 0 // Let other handlers or the default handler process the event
	}
}

var ignoreSignals = []os.Signal{os.Interrupt}
var forwardSignals = []os.Signal{}

// makeShutdownCh creates an interrupt listener and returns a channel.
// A message will be sent on the channel for every interrupt received.
func makeShutdownCh() <-chan struct{} {
	resultCh := make(chan struct{})
	p := unsafe.Pointer(&resultCh)

	ret, _, err := setConsoleCtrlHandler.Call(
		syscall.NewCallback(ConsoleCtrlHandler), // Pointer to our handler function
		uintptr(1),                              // Add the handler (TRUE)
		uintptr(p),
	)

	if ret == 0 || err != nil {
		log.Printf("[ERROR] error setting console ctrl handler: %v", err)
	}

	return resultCh
}
