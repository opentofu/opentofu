// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Get represents the command-line arguments for the get command.
type Get struct {
	// Update is the flag that can be used to upgrade the version of the modules.
	Update bool
	// TestsDirectory indicates the path where the tests are stored
	TestsDirectory string

	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions
}

// BindGet registers CLI arguments, returning a Get value and it's corresponding hooks.
func BindGet(flags Flags) (*Get, Hooks) {
	var arguments Get
	var hooks Hooks

	arguments.ViewOptions.bind(flags, true)
	hooks = append(hooks, arguments.ViewOptions.ParseHook())

	arguments.Vars = &Vars{}
	arguments.Vars.bind(flags)

	flags.BoolVar(&arguments.Update, "update", false, "Check already-downloaded modules for available updates and install the newest versions available.")
	flags.StringVar(&arguments.TestsDirectory, "test-directory", "tests", `Set the OpenTofu test directory, defaults to "tests". When set, the test command will search for test files in the current directory and in the one specified by the flag.`).SetDisplay("=path")

	return &arguments, hooks
}

// ParseGet processes CLI arguments, returning a Get value, a closer function, and errors.
// If errors are encountered, a Get value is still returned representing
// the best effort interpretation of the arguments.
func ParseGet(args []string) (*Get, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	arguments, hooks := BindGet(flags)

	cmdFlags := defaultFlagSet("get", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	if len(cmdFlags.Args()) > 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Unexpected argument",
			"Too many command line arguments. Did you mean to use -chdir?",
		))
	}

	return arguments, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
