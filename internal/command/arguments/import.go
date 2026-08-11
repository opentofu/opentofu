// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Import represents the command-line arguments for the import command.
type Import struct {
	// ResourceAddress is the absolute resource address that the user is required to provide to indicate
	// on which configuration resource the state of the resource needs to be imported.
	ResourceAddress string
	// ResourceID is the platform provided ID of the resource to be imported.
	ResourceID string
	// ConfigPath is the path to the directory where the configuration containing the ResourceAddress is
	// accessible.
	ConfigPath string
	// Parallelism is the limit of concurrent operation as OpenTofu walks the graph
	Parallelism int

	// View represents the global view options
	View *View
	// State, Backend and Vars are the common extended flags
	State   *State
	Backend *Backend
	Vars    *Vars
}

// BindImport registers CLI arguments, returning a Import value and it's corresponding hooks.
func BindImport(cli *CommandLine) *Import {
	ret := Import{
		View:    BindView(cli, viewFlagAll),
		Vars:    BindVars(cli),
		Backend: BindBackend(cli),
		State:   BindState(cli, stateFlagAll),
	}

	cli.IntVar(&ret.Parallelism, "parallelism", DefaultParallelism, `Limit the number of parallel resource operations. Defaults to 10.`).SetDisplay("=n")
	cli.StringVar(&ret.ConfigPath, "config", "", "Path to a directory of OpenTofu configuration files to use to configure the provider. Defaults to pwd. If no config files are present, they must be provided via the input prompts or env vars.").SetDisplay("=path")

	cli.ArgHelp = "The import command expects two arguments"
	cli.PositionalArg(&ret.ResourceAddress, "ADDR", false)
	cli.PositionalArg(&ret.ResourceID, "ID", false)

	return &ret
}

// ParseImport processes CLI arguments, returning an Import value, a closer function, and errors.
// If errors are encountered, an Import value is still returned representing
// the best effort interpretation of the arguments.
func ParseImport(args []string) (*Import, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindImport(cli)
	closer, diags := cli.parseWithHooks("import", args)
	return ret, closer, diags
}
