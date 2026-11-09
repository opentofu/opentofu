// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configload

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type diagnosticExitStatus interface {
	ExitCode() int
}

type ExitCode uint8

func (e ExitCode) ExitCode() int {
	switch e {
	case ExitSuccess, ExitFailure, ExitInitRequired:
		return int(e)
	default:
		return int(ExitFailure)
	}
}

const (
	// ExitStatus is used to indicate the command ended in succes.
	ExitSuccess ExitCode = 0
	// ExitFailure is the result of a command failure.
	ExitFailure ExitCode = 1
	// ExitInitRequired is used to indicate that a command requires
	// `tofu init` to be run
	ExitInitRequired ExitCode = 4
)

// ExitStatus checks if any of the diagnostics includes a
// DiagnosticExitStatus in the ExtraInfo and returns its exit code, or
// returns 1 if the diags have any errors
func ExitStatus(diags tfdiags.Diagnostics) int {
	var exit diagnosticExitStatus = ExitSuccess
	if diags.HasErrors() {
		exit = ExitFailure
	}

	for _, d := range diags {
		if e, ok := d.ExtraInfo().(diagnosticExitStatus); ok {
			exit = e
			break
		}
	}
	return exit.ExitCode()
}
