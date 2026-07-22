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

// BindUnlock registers CLI arguments, returning a Unlock value and it's corresponding hooks.
func BindUnlock(flags Flags) (*Unlock, Hooks) {
	var arguments Unlock
	var hooks Hooks

	arguments.ViewOptions.bind(flags, false)
	hooks = append(hooks, arguments.ViewOptions.ParseHook())

	arguments.Vars = &Vars{}
	arguments.Vars.bind(flags)

	flags.BoolVar(&arguments.Force, "force", false, "Don't ask for input for unlock confirmation.")

	return &arguments, hooks
}

// ParseUnlock processes CLI arguments, returning a Unlock value, a closer function, and errors.
// If errors are encountered, a Unlock value is still returned representing
// the best effort interpretation of the arguments.
func ParseUnlock(args []string) (*Unlock, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	arguments, hooks := BindUnlock(flags)

	cmdFlags := defaultFlagSet("force-unlock", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	// TODO positional args
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

	return arguments, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
