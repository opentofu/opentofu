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

	// View represents the global view options
	View *View
}

// BindLogout registers CLI arguments, returning a Logout value and it's corresponding hooks.
func BindLogout(cli *CommandLine) *Logout {
	arguments := Logout{
		View: BindView(cli, viewFlagNoInput),
	}

	cli.ArgHelp = "The logout command expects exactly one argument: the host to log out of."
	cli.PositionalArg(&arguments.Host, "hostname", false)

	return &arguments
}

// ParseLogout processes CLI arguments, returning a Logout value, a closer function, and errors.
// If errors are encountered, a Logout value is still returned representing
// the best effort interpretation of the arguments.
func ParseLogout(args []string) (*Logout, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	arguments := BindLogout(cli)
	closer, diags := cli.parseWithHooks("logout", args)
	return arguments, closer, diags
}
