// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ProvidersMirror represents the command-line arguments for the 'providers lock' command.
type ProvidersMirror struct {
	// Directory is the directory where the copies of the providers will be stored
	Directory string
	// OptPlatforms contains the platforms that the user requested to have the providers
	// copy for
	OptPlatforms flags.FlagStringSlice

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions
	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
}

// ParseProvidersMirror processes CLI arguments, returning a ProvidersMirror value, a closer function, and errors.
// If errors are encountered, a ProvidersMirror value is still returned representing
// the best effort interpretation of the arguments.
func ParseProvidersMirror(args []string) (*ProvidersMirror, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	arguments := &ProvidersMirror{
		Vars: &Vars{},
	}

	cmdFlags := extendedFlagSet("providers mirror", nil, nil, arguments.Vars)
	cmdFlags.Var(&arguments.OptPlatforms, "platform", "target platform")
	arguments.ViewOptions.AddFlags(cmdFlags, false)
	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}
	remainingArgs := cmdFlags.Args()
	if len(remainingArgs) != 1 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Wrong number of arguments",
			"The providers mirror command requires an output directory as a command-line argument.",
		))
	} else {
		arguments.Directory = remainingArgs[0]
	}

	closer, moreDiags := arguments.ViewOptions.Parse()
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return arguments, closer, diags
	}

	return arguments, closer, diags
}
