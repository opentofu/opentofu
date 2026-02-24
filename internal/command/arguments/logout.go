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
	Host string
	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
}

// ParseLogout processes CLI arguments, returning a Logout value, a closer function, and errors.
// If errors are encountered, a Logout value is still returned representing
// the best effort interpretation of the arguments.
func ParseLogout(args []string) (*Logout, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	arguments := &Logout{}

	cmdFlags := defaultFlagSet("logout")
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

	if len(cmdFlags.Args()) != 1 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Unexpected argument",
			"The logout command expects exactly one argument: the host to log out of.",
		))
		return arguments, closer, diags
	}
	arguments.Host = cmdFlags.Args()[0]
	return arguments, closer, diags
}
