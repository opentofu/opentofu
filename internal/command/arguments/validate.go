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

// ParseValidate processes CLI arguments, returning a Validate value, a closer function, and errors.
// If errors are encountered, a Validate value is still returned representing
// the best effort interpretation of the arguments.
func ParseValidate(args []string) (*Validate, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	validate := &Validate{
		Path: ".",
		Vars: &Vars{},
	}

	cmdFlags := extendedFlagSet("validate", nil, nil, validate.Vars)
	cmdFlags.StringVar(&validate.TestDirectory, "test-directory", "tests", "test-directory")
	cmdFlags.BoolVar(&validate.NoTests, "no-tests", false, "no-tests")

	validate.ViewOptions.AddFlags(cmdFlags, false)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	args = cmdFlags.Args()
	if len(args) > 1 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Too many command line arguments",
			"Expected at most one positional argument.",
		))
	}

	if len(args) > 0 {
		validate.Path = args[0]
	}

	closer, moreDiags := validate.ViewOptions.Parse()
	diags = diags.Append(moreDiags)

	return validate, closer, diags
}
