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

// BindImport registers CLI arguments, returning a Import value and it's corresponding hooks.
func BindImport(flags Flags, wd *workdir.Dir) (*Import, Hooks) {
	var imp Import
	var hooks Hooks

	imp.ViewOptions.bind(flags, true)
	hooks = append(hooks, imp.ViewOptions.ParseHook())

	imp.Vars = &Vars{}
	imp.Vars.bind(flags)

	imp.Backend = &Backend{}
	imp.Backend.bindIgnoreRemoteVersionFlag(flags)

	imp.State = &State{}
	imp.State.bind(flags, stateFlagAll)

	// Get the pwd since its our default -config flag value
	pwd := wd.NormalizePath(wd.RootModuleDir())

	flags.IntVar(&imp.Parallelism, "parallelism", DefaultParallelism, "parallelism")
	flags.StringVar(&imp.ConfigPath, "config", pwd, "path")

	// TODO positional args

	return &imp, hooks
}

// ParseImport processes CLI arguments, returning an Import value, a closer function, and errors.
// If errors are encountered, an Import value is still returned representing
// the best effort interpretation of the arguments.
func ParseImport(args []string, wd *workdir.Dir) (*Import, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	ret, hooks := BindImport(flags, wd)

	cmdFlags := defaultFlagSet("import", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	args = cmdFlags.Args()
	if len(args) != 2 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid number of arguments",
			"The import command expects two arguments",
		))
	} else {
		ret.ResourceAddress = args[0]
		ret.ResourceID = args[1]
	}
	return ret, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
