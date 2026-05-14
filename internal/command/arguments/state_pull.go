// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// StatePull represents the command-line arguments for the 'state pull' command.
type StatePull struct {
	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars are the common extended flags
	Vars *Vars
}

// ParseStatePull processes CLI arguments, returning a StatePull value, a closer function, and errors.
// If errors are encountered, a StatePull value is still returned representing
// the best effort interpretation of the arguments.
func ParseStatePull(args []string) (*StatePull, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret := &StatePull{
		Vars: &Vars{},
	}
	cmdFlags := extendedFlagSet("state pull", nil, ret.Vars)

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

	// we only parse but do not register the views flags since this command does not need it because it already
	// prints the state in json format
	closer, moreDiags := ret.ViewOptions.Parse()
	diags = diags.Append(moreDiags)

	return ret, closer, diags
}
