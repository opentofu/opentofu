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

	// View represents the global view options
	View *View
	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
}

// BindProvidersMirror registers CLI arguments, returning a ProvidersMirror value and it's corresponding hooks.
func BindProvidersMirror(cli *CommandLine) *ProvidersMirror {
	arguments := ProvidersMirror{
		View: BindView(cli, viewFlagNoInput),
		Vars: BindVars(cli),
	}

	cli.StringArrayVar(&arguments.OptPlatforms, "platform", nil, `Choose which target platform to build a mirror for. By default OpenTofu will obtain plugin packages suitable for the platform where you run this command. Use this flag multiple times to include packages for multiple target systems.

 Target names consist of an operating system and a CPU architecture. For example, "linux_amd64" selects the Linux operating system running on an AMD64 or x86_64 CPU. Each provider is available only for a limited set of target platforms.`).SetDisplay("=os_arch")

	cli.ArgHelp = "The providers mirror command requires an output directory as a command-line argument."
	cli.PositionalArg(&arguments.Directory, "target-dir", false)

	return &arguments
}

// ParseProvidersMirror processes CLI arguments, returning a ProvidersMirror value, a closer function, and errors.
// If errors are encountered, a ProvidersMirror value is still returned representing
// the best effort interpretation of the arguments.
func ParseProvidersMirror(args []string) (*ProvidersMirror, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	arguments := BindProvidersMirror(cli)
	closer, diags := cli.parseWithHooks("providers mirror", args)
	return arguments, closer, diags
}
