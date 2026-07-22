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
func BindProvidersMirror(flags Flags) (*ProvidersMirror, Hooks) {
	var arguments ProvidersMirror
	var hooks Hooks

	arguments.ViewOptions.bind(flags, false)
	hooks = append(hooks, arguments.ViewOptions.ParseHook())

	arguments.Vars = &Vars{}
	arguments.Vars.bind(flags)

	flags.StringArrayVar(&arguments.OptPlatforms, "platform", nil, "target platform")

	return &arguments, hooks
}

// ParseProvidersMirror processes CLI arguments, returning a ProvidersMirror value, a closer function, and errors.
// If errors are encountered, a ProvidersMirror value is still returned representing
// the best effort interpretation of the arguments.
func ParseProvidersMirror(args []string) (*ProvidersMirror, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	arguments, hooks := BindProvidersMirror(flags)

	cmdFlags := defaultFlagSet("providers mirror", flags)
	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	// TODO positional arguments
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

	return arguments, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
