// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Version represents the command-line arguments for the version command.
type Version struct {
	ViewOptions ViewOptions
}

// ParseVersion processes CLI arguments, returning a Version value, a closer function, and errors.
// If errors are encountered, a Version value is still returned representing
// the best effort interpretation of the arguments.
func ParseVersion(args []string) (*Version, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	ret := &Version{}

	cmdFlags := defaultFlagSet("version")
	// Enable but ignore the global version flags. In main.go, if any of the
	// arguments are -v, -version, or --version, this command will be called
	// with the rest of the arguments, so we need to be able to cope with
	// those.
	cmdFlags.Bool("v", true, "version")
	cmdFlags.Bool("version", true, "version")
	ret.ViewOptions.AddGranularFlags(cmdFlags, false, false) // Add only the -json flag

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	closer, moreDiags := ret.ViewOptions.Parse()
	diags = diags.Append(moreDiags)
	return ret, closer, diags
}
