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
func BindProviders(flags Flags) (*Providers, Hooks) {
	var providers Providers
	var hooks Hooks

	// we only parse but do not register the views flags since this command does not need it
	hooks = append(hooks, providers.ViewOptions.ParseHook())

	providers.Vars = &Vars{}
	providers.Vars.bind(flags)

	flags.StringVar(&providers.TestsDirectory, "test-directory", "tests", `Set the OpenTofu test directory, defaults to "tests". When set, the test command will search for test files in the current directory and in the one specified by the flag.`).SetDisplay("=path")

	return &providers, hooks
}

// ParseProviders processes CLI arguments, returning a Providers value, a closer function, and errors.
// If errors are encountered, a Providers value is still returned representing
// the best effort interpretation of the arguments.
func ParseProviders(args []string) (*Providers, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	arguments, hooks := BindProviders(flags)

	cmdFlags := defaultFlagSet("providers", flags)

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
