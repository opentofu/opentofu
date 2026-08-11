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

	// View represents the global view options
	View *View
	// Vars and State are the common extended flags
	Vars  *Vars
	State *State
}

// BindLogin registers CLI arguments, returning a Login value and it's corresponding hooks.
func BindLogin(cli *CommandLine) *Login {
	arguments := Login{
		View: BindView(cli, viewFlagAll),
		// Even though the command does not use the -var/-var-file content, we will keep this for the moment
		// just to keep backwards compatibility for users (in case any of them are using these flags with this command)
		Vars: BindVars(cli),
	}

	// State is only initialised and no flags are registered since the login command needs to lock the
	// state by default, with no user input on that.
	arguments.State = &State{Lock: true}

	cli.ArgHelp = "The login command expects exactly one argument: the host to log in to."
	cli.PositionalArg(&arguments.Host, "hostname", false)

	return &arguments
}

// ParseLogin processes CLI arguments, returning a Login value, a closer function, and errors.
// If errors are encountered, a Login value is still returned representing
// the best effort interpretation of the arguments.
func ParseLogin(args []string) (*Login, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	arguments := BindLogin(cli)
	closer, diags := cli.parseWithHooks("login", args)
	return arguments, closer, diags
}
