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
	"testing"
	"time"
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

func handleSignals(t *testing.T, resultCh chan struct{}) (func(t *testing.T), error) {
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, syscall.SIGINT, os.Interrupt)

	select {
	case <-sigint:
		resultCh <- struct{}{}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected lock timeout after 50 milliseconds, waited 500 milliseconds")
	}

	unregisterFn := func(t *testing.T) {
		signal.Stop(sigint)
	}

	return unregisterFn, nil
}
