// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !windows
// +build !windows

package remote

import (
	"fmt"
	"os"
	"syscall"
)

// sendInterruptSignal sends an SIGINT signal to the given process ID.
func sendInterruptSignal(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("[ERROR] error searching process ID: %v", err)
	}
	if err := p.Signal(syscall.SIGINT); err != nil {
		return fmt.Errorf("[ERROR] error sending interrupt signal: %v", err)
	}
	return nil
}
