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
	ViewOptions ViewOptions
}

// ParseMetadataFunctions processes CLI arguments, returning a MetadataFunctions value, a closer function, and errors.
// If errors are encountered, a MetadataFunctions value is still returned representing
// the best effort interpretation of the arguments.
func ParseMetadataFunctions(args []string) (*MetadataFunctions, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	arguments := &MetadataFunctions{}

	cmdFlags := defaultFlagSet("metadata functions")
	arguments.ViewOptions.AddGranularFlags(cmdFlags, false, false) // Add only the -json flag

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

	if arguments.ViewOptions.ViewType != ViewJSON {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid arguments",
			"The `tofu metadata functions` command requires the `-json` flag.",
		))
	}
	return arguments, closer, diags
}
