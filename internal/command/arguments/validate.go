// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Validate represents the command-line arguments for the validate command.
type Validate struct {
	// Path is the directory containing the configuration to be validated. If
	// unspecified, validate will use the current directory.
	Path string

	// TestDirectory is the directory containing any test files that should be
	// validated alongside the main configuration. Should be relative to the
	// Path.
	TestDirectory string

	// NoTests indicates that OpenTofu should not validate any test files
	// included with the module.
	NoTests bool

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	Vars *Vars
}

// BindValidate registers CLI arguments, returning a Validate value and it's corresponding hooks.
func BindValidate(flags Flags) (*Validate, Hooks) {
	var validate Validate
	var hooks Hooks

	validate.ViewOptions.bind(flags, false)
	hooks = append(hooks, validate.ViewOptions.ParseHook())

	validate.Vars = &Vars{}
	validate.Vars.bind(flags)

	flags.StringVar(&validate.TestDirectory, "test-directory", "tests", `Set the OpenTofu test directory, defaults to "tests". When set, the test command will search for test files in the current directory and in the one specified by the flag.`).SetDisplay("=path")
	flags.BoolVar(&validate.NoTests, "no-tests", false, "If specified, OpenTofu will not validate test files.")

	return &validate, hooks
}

// ParseValidate processes CLI arguments, returning a Validate value, a closer function, and errors.
// If errors are encountered, a Validate value is still returned representing
// the best effort interpretation of the arguments.
func ParseValidate(args []string) (*Validate, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	validate, hooks := BindValidate(flags)

	cmdFlags := defaultFlagSet("validate", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	// TODO positional arguments
	args = cmdFlags.Args()
	if len(args) > 1 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Too many command line arguments",
			"Expected at most one positional argument.",
		))
	}

	validate.Path = "."
	if len(args) > 0 {
		validate.Path = args[0]
	}

	return validate, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
