// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// MetadataFunctions represents the command-line arguments for the "metadata functions" command.
type MetadataFunctions struct {
	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions
}

// BindMetadataFunctions registers CLI arguments, returning a MetadataFunctions value and it's corresponding hooks.
func BindMetadataFunctions(flags Flags) (*MetadataFunctions, Hooks) {
	var arguments MetadataFunctions
	var hooks Hooks

	arguments.ViewOptions.bindGranularFlags(flags, false, false) // Add only the -json flag
	hooks = append(hooks, arguments.ViewOptions.ParseHook())

	hooks = append(hooks, Hook{Pre: func() tfdiags.Diagnostics {
		var diags tfdiags.Diagnostics

		// The 'metadata functions' command just forces the user to use the `-json` flag but any of the diagnostics should
		// be printed as human format. This makes it clear that the success output of this command will be in json and
		// that it needs to be processed accordingly.
		// The print of the functions will be in JSON all the time.
		if arguments.ViewOptions.ViewType != ViewJSON {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid arguments",
				"The `tofu metadata functions` command requires the `-json` flag.",
			))
		}
		arguments.ViewOptions.ViewType = ViewHuman

		return diags
	}})

	return &arguments, hooks
}

// ParseMetadataFunctions processes CLI arguments, returning a MetadataFunctions value, a closer function, and errors.
// If errors are encountered, a MetadataFunctions value is still returned representing
// the best effort interpretation of the arguments.
func ParseMetadataFunctions(args []string) (*MetadataFunctions, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	arguments, hooks := BindMetadataFunctions(flags)

	cmdFlags := defaultFlagSet("metadata functions", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	return arguments, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
