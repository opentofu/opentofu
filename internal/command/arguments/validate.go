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

	// View represents the global view options
	View *View

	Vars *Vars
}

// BindValidate registers CLI arguments, returning a Validate value and it's corresponding hooks.
func BindValidate(cli *CommandLine) *Validate {
	validate := Validate{
		View: BindView(cli, viewFlagNoInput),
		Vars: BindVars(cli),
	}

	cli.StringVar(&validate.TestDirectory, "test-directory", "tests", `Set the OpenTofu test directory, defaults to "tests". When set, the test command will search for test files in the current directory and in the one specified by the flag.`).SetDisplay("=path")
	cli.BoolVar(&validate.NoTests, "no-tests", false, "If specified, OpenTofu will not validate test files.")

	validate.Path = "."
	cli.PositionalArg(&validate.Path, "path", true)

	return &validate
}

// ParseValidate processes CLI arguments, returning a Validate value, a closer function, and errors.
// If errors are encountered, a Validate value is still returned representing
// the best effort interpretation of the arguments.
func ParseValidate(args []string) (*Validate, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	validate := BindValidate(cli)
	closer, diags := cli.parseWithHooks("validate", args)
	return validate, closer, diags
}
