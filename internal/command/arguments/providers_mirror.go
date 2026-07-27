// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ProvidersMirror represents the command-line arguments for the 'providers lock' command.
type ProvidersMirror struct {
	// Directory is the directory where the copies of the providers will be stored
	Directory string
	// OptPlatforms contains the platforms that the user requested to have the providers
	// copy for
	OptPlatforms []string

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions
	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
}

// BindProvidersMirror registers CLI arguments, returning a ProvidersMirror value and it's corresponding hooks.
func BindProvidersMirror(cli *CommandLine) *ProvidersMirror {
	var arguments ProvidersMirror

	arguments.ViewOptions.bind(cli, false)

	arguments.Vars = &Vars{}
	arguments.Vars.bind(cli)

	cli.StringArrayVar(&arguments.OptPlatforms, "platform", nil, "target platform")

	cli.ArgHelp = "The providers mirror command requires an output directory as a command-line argument."
	cli.PositionalArg(&arguments.Directory, "directory", false)

	return &arguments
}

// ParseProvidersMirror processes CLI arguments, returning a ProvidersMirror value, a closer function, and errors.
// If errors are encountered, a ProvidersMirror value is still returned representing
// the best effort interpretation of the arguments.
func ParseProvidersMirror(args []string) (*ProvidersMirror, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	arguments := BindProvidersMirror(cli)
	closer, diags := cli.Stdlib("providers mirror", args)
	return arguments, closer, diags
}
