// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ProvidersLock represents the command-line arguments for the 'providers lock' command.
type ProvidersLock struct {
	// Providers are the source addresses of the providers that are requested to be updated
	Providers []string
	// OptPlatforms contains the platforms that the user requested for the locks to be updated for.
	// Having this empty, only the checksum for the host platform will be updated, but the user
	// can use this to update the hashes for other platforms too.
	OptPlatforms flags.FlagStringSlice
	// FsMirrorDir represents a path from where OpenTofu should check for providers instead to reach
	// out for the registry.
	FsMirrorDir string
	// NetMirrorURL represents a URL to a mirrored registry from where OpenTofu should check for
	// providers instead to reach out for the registry.
	NetMirrorURL string

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions
	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
}

// ParseProvidersLock processes CLI arguments, returning a ProvidersLock value, a closer function, and errors.
// If errors are encountered, a ProvidersLock value is still returned representing
// the best effort interpretation of the arguments.
func ParseProvidersLock(args []string) (*ProvidersLock, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	arguments := &ProvidersLock{
		Vars: &Vars{},
	}

	cmdFlags := extendedFlagSet("providers lock", nil, nil, arguments.Vars)
	cmdFlags.Var(&arguments.OptPlatforms, "platform", "target platform")
	cmdFlags.StringVar(&arguments.FsMirrorDir, "fs-mirror", "", "filesystem mirror directory")
	cmdFlags.StringVar(&arguments.NetMirrorURL, "net-mirror", "", "network mirror base URL")
	arguments.ViewOptions.AddFlags(cmdFlags, false)
	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}
	if arguments.FsMirrorDir != "" && arguments.NetMirrorURL != "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid installation method options",
			"The -fs-mirror and -net-mirror command line options are mutually-exclusive.",
		))
	}

	closer, moreDiags := arguments.ViewOptions.Parse()
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return arguments, closer, diags
	}
	arguments.Providers = cmdFlags.Args()

	return arguments, closer, diags
}
