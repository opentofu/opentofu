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
	// View represents the global view options
	View *View
}

// BindGet registers CLI arguments, returning a Get value and it's corresponding hooks.
func BindGet(cli *CommandLine) *Get {
	arguments := Get{
		View: BindView(cli, viewFlagNoInput),
		Vars: BindVars(cli),
	}

	cli.BoolVar(&arguments.Update, "update", false, "Check already-downloaded modules for available updates and install the newest versions available.")
	cli.StringVar(&arguments.TestsDirectory, "test-directory", "tests", `Set the OpenTofu test directory, defaults to "tests". When set, the test command will search for test files in the current directory and in the one specified by the flag.`).SetDisplay("=path")

	return &arguments
}

// ParseGet processes CLI arguments, returning a Get value, a closer function, and errors.
// If errors are encountered, a Get value is still returned representing
// the best effort interpretation of the arguments.
func ParseGet(args []string) (*Get, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	arguments := BindGet(cli)
	closer, diags := cli.parseWithHooks("get", args)
	return arguments, closer, diags
}
