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

	// View represents the global view options
	View *View
	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
}

// BindProvidersLock registers CLI arguments, returning a ProvidersLock value and it's corresponding hooks.
func BindProvidersLock(cli *CommandLine) *ProvidersLock {
	arguments := ProvidersLock{
		View: BindView(cli, viewFlagNoInput),
		Vars: BindVars(cli),
	}

	cli.StringArrayVar(&arguments.OptPlatforms, "platform", nil, `Choose a target platform to request package checksums for.

By default OpenTofu will request package checksums suitable only for the platform where you run this command. Use this option multiple times to include checksums for multiple target systems.

Target names consist of an operating system and a CPU architecture. For example, "linux_amd64" selects the Linux operating system running on an AMD64 or x86_64 CPU. Each provider is available only for a limited set of target platforms.`).SetDisplay("=os_arch")
	cli.StringVar(&arguments.FsMirrorDir, "fs-mirror", "", `Consult the given filesystem mirror directory instead of the origin registry for each of the given providers.

This would be necessary to generate lock file entries for a provider that is available only via a mirror, and not published in an upstream registry. In this case, the set of valid checksums will be limited only to what OpenTofu can learn from the data in the mirror directory.`).SetDisplay("=dir")
	cli.StringVar(&arguments.NetMirrorURL, "net-mirror", "", `Consult the given network mirror (given as a base URL) instead of the origin registry for each of the given providers.

This would be necessary to generate lock file entries for a provider that is available only via a mirror, and not published in an upstream registry. In this case, the set of valid checksums will be limited only to what OpenTofu can learn from the data in the mirror indices.`).SetDisplay("=url")
	cli.StringVar(&arguments.OciMirrorTemplate, "oci-mirror", "", `Consult the given OCI registry mirror (given as a template) instead of the origin registry for each of the given providers.

This would be necessary to generate lock file entries for a provider that is available only via an OCI mirror, and not published in an upstream registry.

The argument is a Level 1 URI template as defined by RFC 6570, used to map provider source addresses to OCI repository addresses. The template can contain {hostname} {namespace} and {type}.`).SetDisplay("=tmpl")

	cli.VariadicArg(&arguments.Providers, "providers")

	cli.PreHook(func() tfdiags.Diagnostics {
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
	})

	return &arguments
}

// ParseProvidersLock processes CLI arguments, returning a ProvidersLock value, a closer function, and errors.
// If errors are encountered, a ProvidersLock value is still returned representing
// the best effort interpretation of the arguments.
func ParseProvidersLock(args []string) (*ProvidersLock, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	arguments := BindProvidersLock(cli)
	closer, diags := cli.parseWithHooks("providers lock", args)
	return arguments, closer, diags
}
