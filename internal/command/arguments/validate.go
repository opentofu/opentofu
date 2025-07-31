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

	// ViewType specifies which output format to use: human, JSON, or "raw".
	ViewType ViewType

	Vars *Vars
}

// ParseValidate processes CLI arguments, returning a Validate value and errors.
// If errors are encountered, a Validate value is still returned representing
// the best effort interpretation of the arguments.
func ParseValidate(args []string) (*Validate, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	validate := &Validate{
		Path: ".",
		Vars: &Vars{},
	}

	var jsonOutput bool
	cmdFlags := extendedFlagSet("validate", nil, nil, validate.Vars)
	cmdFlags.BoolVar(&jsonOutput, "json", false, "json")
	cmdFlags.StringVar(&validate.TestDirectory, "test-directory", "tests", "test-directory")
	cmdFlags.BoolVar(&validate.NoTests, "no-tests", false, "no-tests")

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

	switch {
	case jsonOutput:
		validate.ViewType = ViewJSON
	default:
		validate.ViewType = ViewHuman
	}

	return validate, diags
}
