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
	// View represents the global view options
	View *View
}

// BindMetadataFunctions registers CLI arguments, returning a MetadataFunctions value and it's corresponding hooks.
func BindMetadataFunctions(cli *CommandLine) *MetadataFunctions {
	arguments := MetadataFunctions{
		View: BindView(cli, viewFlagJson),
	}

	cli.PreHook(func() tfdiags.Diagnostics {
		var diags tfdiags.Diagnostics

		// The 'metadata functions' command just forces the user to use the `-json` flag but any of the diagnostics should
		// be printed as human format. This makes it clear that the success output of this command will be in json and
		// that it needs to be processed accordingly.
		// The print of the functions will be in JSON all the time.
		if arguments.View.ViewType != ViewJSON {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid arguments",
				"The `tofu metadata functions` command requires the `-json` flag.",
			))
		}
		arguments.View.ViewType = ViewHuman

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
	closer, diags := cli.parseWithHooks("metadata functions", args)
	return arguments, closer, diags
}
