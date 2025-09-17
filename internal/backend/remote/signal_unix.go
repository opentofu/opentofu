// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !windows
// +build !windows

package remote

import (
	"os"
	"os/signal"
	"syscall"
)

// sendInterruptSignal sends an SIGINT signal to the given process ID.
func sendInterruptSignal(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := p.Signal(syscall.SIGINT); err != nil {
		return err
	}
	return nil
}

func listenForConsoleCtrlHandler(resultCh chan struct{}) {
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, syscall.SIGINT, os.Interrupt)
}

func stopCtrlHandler(resultCh chan struct{}) {
	sigint := make(chan os.Signal, 1)
	signal.Stop(sigint)
}
