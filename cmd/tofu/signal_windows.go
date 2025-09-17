// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows
// +build windows

package main

import (
	"fmt"
	"log"
	"os"
	"syscall"
	"time"
)

// Define the handler function signature for SetConsoleCtrlHandler
type HandlerRoutine func(dwCtrlType uint32) uintptr

// SetConsoleCtrlHandler function from kernel32.dll
var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	setConsoleCtrlHandler = kernel32.NewProc("SetConsoleCtrlHandler")
)

// ConsoleCtrlHandler is our custom handler routine
func ConsoleCtrlHandler(dwCtrlType uint32) uintptr {
	switch dwCtrlType {
	case syscall.CTRL_C_EVENT:
		fmt.Println("\nReceived CTRL+C event. Performing cleanup...")
		// Perform any necessary cleanup here
		time.Sleep(1 * time.Second) // Simulate cleanup
		fmt.Println("Cleanup complete. Exiting.")
		os.Exit(0) // Exit gracefully
		return 1   // Indicate that the event was handled
	case syscall.CTRL_CLOSE_EVENT:
		fmt.Println("\nReceived CTRL+CLOSE event. Performing cleanup...")
		time.Sleep(1 * time.Second)
		fmt.Println("Cleanup complete. Exiting.")
		os.Exit(0)
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

	ret, _, err := setConsoleCtrlHandler.Call(
		syscall.NewCallback(ConsoleCtrlHandler), // Pointer to our handler function
		uintptr(1),                              // Add the handler (TRUE)
	)

	if ret == 0 || err != nil {
		log.Printf("[ERROR] error setting console ctrl handler: %v", err)
	}

	return resultCh
}
