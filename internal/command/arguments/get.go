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
	Update         bool
	TestsDirectory string

	Vars        *Vars
	ViewOptions ViewOptions
}

// ParseGet processes CLI arguments, returning a Get value, a closer function, and errors.
// If errors are encountered, a Get value is still returned representing
// the best effort interpretation of the arguments.
func ParseGet(args []string) (*Get, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	arguments := &Get{
		Vars: &Vars{},
	}

	cmdFlags := extendedFlagSet("get", nil, nil, arguments.Vars)
	cmdFlags.BoolVar(&arguments.Update, "update", false, "update")
	cmdFlags.StringVar(&arguments.TestsDirectory, "test-directory", "tests", "test-directory")
	arguments.ViewOptions.AddFlags(cmdFlags, false)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	closer, moreDiags := arguments.ViewOptions.Parse()
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return arguments, closer, diags
	}
	if len(cmdFlags.Args()) > 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Unexpected argument",
			"Too many command line arguments. Did you mean to use -chdir?",
		))
	}

	return arguments, closer, diags
}
