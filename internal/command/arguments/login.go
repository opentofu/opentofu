// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Login represents the command-line arguments for the login command.
type Login struct {
	// Host represents the host that OpenTofu will try to login to
	Host string

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions
	// Vars and State are the common extended flags
	Vars  *Vars
	State *State
}

// BindLogin registers CLI arguments, returning a Login value and it's corresponding hooks.
func BindLogin(flags Flags) (*Login, Hooks) {
	var login Login
	var hooks Hooks

	login.ViewOptions.bind(flags, true)
	hooks = append(hooks, login.ViewOptions.ParseHook())

	// Even though the command does not use the -var/-var-file content, we will keep this for the moment
	// just to keep backwards compatibility for users (in case any of them are using these flags with this command)
	login.Vars = &Vars{}
	login.Vars.bind(flags)

	// State is only initialised and no flags are registered since the login command needs to lock the
	// state by default, with no user input on that.
	login.State = &State{Lock: true}

	// TODO positional arguments

	return &login, hooks
}

// ParseLogin processes CLI arguments, returning a Login value, a closer function, and errors.
// If errors are encountered, a Login value is still returned representing
// the best effort interpretation of the arguments.
func ParseLogin(args []string) (*Login, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	arguments, hooks := BindLogin(flags)

	cmdFlags := defaultFlagSet("login", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	if len(cmdFlags.Args()) != 1 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Unexpected argument",
			"The login command expects exactly one argument: the host to log in to.",
		))
	} else {
		arguments.Host = cmdFlags.Args()[0]
	}

	return arguments, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
