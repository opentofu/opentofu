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
	TestsDirectory string

	Vars        *Vars
	ViewOptions ViewOptions
}

// ParseProviders processes CLI arguments, returning a Providers value, a closer function, and errors.
// If errors are encountered, a Providers value is still returned representing
// the best effort interpretation of the arguments.
func ParseProviders(args []string) (*Providers, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	arguments := &Providers{
		Vars: &Vars{},
	}

	cmdFlags := extendedFlagSet("providers", nil, nil, arguments.Vars)
	cmdFlags.StringVar(&arguments.TestsDirectory, "test-directory", "tests", "test-directory")

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	// we only parse but do not register the views flags since this command does not need it
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
