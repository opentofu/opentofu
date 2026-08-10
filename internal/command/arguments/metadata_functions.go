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
func BindMetadataFunctions(cli *CommandLine) *MetadataFunctions {
	var arguments MetadataFunctions

	arguments.ViewOptions.bindGranularFlags(cli, false, false) // Add only the -json flag

	cli.PreHook(func() tfdiags.Diagnostics {
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
	})

	return &arguments
}

// ParseMetadataFunctions processes CLI arguments, returning a MetadataFunctions value, a closer function, and errors.
// If errors are encountered, a MetadataFunctions value is still returned representing
// the best effort interpretation of the arguments.
func ParseMetadataFunctions(args []string) (*MetadataFunctions, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	arguments := BindMetadataFunctions(cli)
	closer, diags := cli.Stdlib("metadata functions", args)
	return arguments, closer, diags
}
