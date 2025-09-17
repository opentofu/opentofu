// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows
// +build windows

package remote

import (
	"fmt"
	"syscall"
)

// sendInterruptSignal sends an Ctrl+Break event to the given process ID.
func sendInterruptSignal(pid int) error {
	d, e := syscall.LoadDLL("kernel32.dll")
	if e != nil {
		return fmt.Errorf("LoadDLL: %v\n", e)
	}
	p, e := d.FindProc("GenerateConsoleCtrlEvent")
	if e != nil {
		return fmt.Errorf("FindProc: %v\n", e)
	}
	r, _, e := p.Call(syscall.CTRL_BREAK_EVENT, uintptr(pid))
	if r == 0 {
		return fmt.Errorf("GenerateConsoleCtrlEvent: %v\n", e)
	}
	return nil
}
