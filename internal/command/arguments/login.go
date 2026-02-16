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
	Host string
	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
}

// ParseLogin processes CLI arguments, returning a Login value, a closer function, and errors.
// If errors are encountered, a Login value is still returned representing
// the best effort interpretation of the arguments.
func ParseLogin(args []string) (*Login, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	arguments := &Login{
		// Even though the command does not use the -var/-var-file content, we will keep this for the moment
		// just to keep backwards compatibility for users (in case any of them are using these flags with this command)
		Vars: &Vars{},
	}

	cmdFlags := extendedFlagSet("login", nil, nil, arguments.Vars)
	arguments.ViewOptions.AddFlags(cmdFlags, true)
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
			"The login command expects exactly one argument: the host to log in to.",
		))
		return arguments, closer, diags
	}
	arguments.Host = cmdFlags.Args()[0]
	return arguments, closer, diags
}
