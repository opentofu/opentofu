// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/command/workdir"
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

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions
	// State, Backend and Vars are the common extended flags
	State   *State
	Backend *Backend
	Vars    *Vars
}

// ParseImport processes CLI arguments, returning an Import value, a closer function, and errors.
// If errors are encountered, an Import value is still returned representing
// the best effort interpretation of the arguments.
func ParseImport(args []string, wd *workdir.Dir) (*Import, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	ret := &Import{
		Vars:    &Vars{},
		State:   &State{},
		Backend: &Backend{},
	}
	// Get the pwd since its our default -config flag value
	pwd := wd.NormalizePath(wd.RootModuleDir())

	cmdFlags := extendedFlagSet("import", ret.State, nil, ret.Vars)
	ret.Backend.AddIgnoreRemoteVersionFlag(cmdFlags)
	cmdFlags.IntVar(&ret.Parallelism, "parallelism", DefaultParallelism, "parallelism")
	cmdFlags.StringVar(&ret.ConfigPath, "config", pwd, "path")
	ret.ViewOptions.AddFlags(cmdFlags, true)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	closer, moreDiags := ret.ViewOptions.Parse()
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return ret, closer, diags
	}

	args = cmdFlags.Args()
	if len(args) != 2 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid number of arguments",
			"The import command expects two arguments",
		))
		return ret, closer, diags
	}
	ret.ResourceAddress = args[0]
	ret.ResourceID = args[1]
	return ret, closer, diags
}
