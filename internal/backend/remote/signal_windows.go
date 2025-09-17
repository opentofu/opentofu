// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows
// +build windows

package remote

import (
	"syscall"

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
