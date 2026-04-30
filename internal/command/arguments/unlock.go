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
	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions
}

// ParseUnlock processes CLI arguments, returning a Unlock value, a closer function, and errors.
// If errors are encountered, a Unlock value is still returned representing
// the best effort interpretation of the arguments.
func ParseUnlock(args []string) (*Unlock, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	arguments := &Unlock{
		Vars: &Vars{},
	}

	cmdFlags := extendedFlagSet("force-unlock", nil, arguments.Vars)
	cmdFlags.BoolVar(&arguments.Force, "force", false, "force")
	arguments.ViewOptions.AddFlags(cmdFlags, false)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	closer, moreDiags := arguments.ViewOptions.Parse()
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return arguments, closer, diags
	}
	args = cmdFlags.Args()
	if len(args) != 1 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Wrong number of arguments",
			"Expected a single argument: LOCK_ID",
		))
	} else {
		arguments.LockID = args[0]
	}

	return arguments, closer, diags
}
