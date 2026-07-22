// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Logout represents the command-line arguments for the logout command.
type Logout struct {
	// Host represents the host that OpenTofu will try to log out of
	Host string

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions
	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
}

// BindLogout registers CLI arguments, returning a Logout value and it's corresponding hooks.
func BindLogout(flags Flags) (*Logout, Hooks) {
	var arguments Logout
	var hooks Hooks

	arguments.ViewOptions.bind(flags, false)
	hooks = append(hooks, arguments.ViewOptions.ParseHook())

	return &arguments, hooks
}

// ParseLogout processes CLI arguments, returning a Logout value, a closer function, and errors.
// If errors are encountered, a Logout value is still returned representing
// the best effort interpretation of the arguments.
func ParseLogout(args []string) (*Logout, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	arguments, hooks := BindLogout(flags)

	cmdFlags := defaultFlagSet("logout", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	// TODO positional args
	if len(cmdFlags.Args()) != 1 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Unexpected argument",
			"The logout command expects exactly one argument: the host to log out of.",
		))
	} else {
		arguments.Host = cmdFlags.Args()[0]
	}

	return arguments, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
