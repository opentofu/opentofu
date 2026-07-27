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
	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions
}

// BindProviders registers CLI arguments, returning a Providers value and it's corresponding hooks.
func BindProviders(cli *CommandLine) *Providers {
	var providers Providers

	// we only parse but do not register the views flags since this command does not need it
	providers.ViewOptions.ParseHook(cli)

	providers.Vars = &Vars{}
	providers.Vars.bind(cli)

	cli.StringVar(&providers.TestsDirectory, "test-directory", "tests", `Set the OpenTofu test directory, defaults to "tests". When set, the test command will search for test files in the current directory and in the one specified by the flag.`).SetDisplay("=path")

	return &providers
}

// ParseProviders processes CLI arguments, returning a Providers value, a closer function, and errors.
// If errors are encountered, a Providers value is still returned representing
// the best effort interpretation of the arguments.
func ParseProviders(args []string) (*Providers, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	arguments := BindProviders(cli)
	closer, diags := cli.Stdlib("providers", args)
	return arguments, closer, diags
}
