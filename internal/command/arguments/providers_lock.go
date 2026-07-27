// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/svchost/uritemplates"
)

// ProvidersLock represents the command-line arguments for the 'providers lock' command.
type ProvidersLock struct {
	// Providers are the source addresses of the providers that are requested to be updated
	Providers []string
	// OptPlatforms contains the platforms that the user requested for the locks to be updated for.
	// Having this empty, only the checksum for the host platform will be updated, but the user
	// can use this to update the hashes for other platforms too.
	OptPlatforms []string
	// FsMirrorDir represents a path from where OpenTofu should check for providers instead to reach
	// out for the registry.
	FsMirrorDir string
	// NetMirrorURL represents a URL to a mirrored registry from where OpenTofu should check for
	// providers instead to reach out for the registry.
	NetMirrorURL string
	// OciMirrorTemplate represents a URI-style template string that evaluates to an OCI repository address
	// to use for the providers.
	OciMirrorTemplate string

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions
	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
}

// BindProvidersLock registers CLI arguments, returning a ProvidersLock value and it's corresponding hooks.
func BindProvidersLock(cli *CommandLine) *ProvidersLock {
	var arguments ProvidersLock

	arguments.ViewOptions.bind(cli, false)

	arguments.Vars = &Vars{}
	arguments.Vars.bind(cli)

	cli.StringArrayVar(&arguments.OptPlatforms, "platform", nil, "target platform")
	cli.StringVar(&arguments.FsMirrorDir, "fs-mirror", "", "filesystem mirror directory")
	cli.StringVar(&arguments.NetMirrorURL, "net-mirror", "", "network mirror base URL")
	cli.StringVar(&arguments.OciMirrorTemplate, "oci-mirror", "", "oci mirror URI template")

	cli.VariadicArg(&arguments.Providers, "providers")

	cli.Hook(Hook{Pre: func() tfdiags.Diagnostics {
		var diags tfdiags.Diagnostics

		mirrorSet := false
		for _, mirror := range []string{arguments.FsMirrorDir, arguments.NetMirrorURL, arguments.OciMirrorTemplate} {
			if mirror == "" {
				continue
			}
			if mirrorSet {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Invalid installation method options",
					"The mirror command line options are mutually-exclusive.",
				))
				break
			}
			mirrorSet = true
		}

		if err := uritemplates.ValidateLevel1(arguments.OciMirrorTemplate); err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid OCI mirror URI template",
				fmt.Sprintf("The -oci-mirror argument is not a valid URI template: %s.", tfdiags.FormatError(err)),
			))
		}

		return diags
	}})

	return &arguments
}

// ParseProvidersLock processes CLI arguments, returning a ProvidersLock value, a closer function, and errors.
// If errors are encountered, a ProvidersLock value is still returned representing
// the best effort interpretation of the arguments.
func ParseProvidersLock(args []string) (*ProvidersLock, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	arguments := BindProvidersLock(cli)
	closer, diags := cli.Stdlib("providers lock", args)
	return arguments, closer, diags
}
