// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import "github.com/mitchellh/cli"

// PedanticCommand is the interface which defines the required functions for the pedantic mode functionality
type PedanticCommand interface {
	cli.Command
	warningFlagged() bool
}

// PedanticRunner is a run wrapper for commands which implement the PedanticCommand interface
type PedanticRunner struct {
	PedanticCommand
}

// Run runs the command returning the appropriate code based on whether a warning has been flagged
func (pr *PedanticRunner) Run(args []string) int {
	retCode := pr.PedanticCommand.Run(args)
	if pr.warningFlagged() {
		retCode = 1
	}
	return retCode
}

// pedanticUI is a UI implementation which directs warning messages to the error output stream
type pedanticUI struct {
	cli.Ui
	notifyWarning func()
}

// Warn sends the warning to stderr and notifies of the warning
func (pui *pedanticUI) Warn(msg string) {
	pui.Ui.Error(msg)
	pui.notifyWarning()
}
