// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Providers represents the command-line arguments for the providers command.
type Providers struct {
	// TestsDirectory indicates the path where the tests are stored
	TestsDirectory string

	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
	// View represents the global view options
	View *View
}

// BindProviders registers CLI arguments, returning a Providers value and it's corresponding hooks.
func BindProviders(cli *CommandLine) *Providers {
	arguments := Providers{
		View: BindView(cli, viewFlagNone),
		Vars: BindVars(cli),
	}

	cli.StringVar(&arguments.TestsDirectory, "test-directory", "tests", `Set the OpenTofu test directory, defaults to "tests". When set, the test command will search for test files in the current directory and in the one specified by the flag.`).SetDisplay("=path")

	return &arguments
}

// ParseProviders processes CLI arguments, returning a Providers value, a closer function, and errors.
// If errors are encountered, a Providers value is still returned representing
// the best effort interpretation of the arguments.
func ParseProviders(args []string) (*Providers, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	arguments := BindProviders(cli)
	closer, diags := cli.parseWithHooks("providers", args)
	return arguments, closer, diags
}
