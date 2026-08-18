// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Unlock represents the command-line arguments for the unlock command.
type Unlock struct {
	// LockID is the ID of the lock that the user has to provide.
	LockID string
	// Force disables the confirmation prompt
	Force bool

	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
	// View represents the global view options
	View *View
}

// BindUnlock registers CLI arguments, returning a Unlock value and it's corresponding hooks.
func BindUnlock(cli *CommandLine) *Unlock {
	arguments := Unlock{
		View: BindView(cli, viewFlagNoInput),
		Vars: BindVars(cli),
	}

	cli.BoolVar(&arguments.Force, "force", false, "Don't ask for input for unlock confirmation.")

	cli.ArgHelp = "Expected a single argument: LOCK_ID"
	cli.PositionalArg(&arguments.LockID, "LOCK_ID", false)

	return &arguments
}

// ParseUnlock processes CLI arguments, returning a Unlock value, a closer function, and errors.
// If errors are encountered, a Unlock value is still returned representing
// the best effort interpretation of the arguments.
func ParseUnlock(args []string) (*Unlock, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	arguments := BindUnlock(cli)
	closer, diags := cli.parseWithHooks("force-unlock", args)
	return arguments, closer, diags
}
