// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Refresh represents the command-line arguments for the apply command.
type Refresh struct {
	// State, Operation, and Vars are the common extended flags
	State     *State
	Operation *Operation
	Vars      *Vars

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions
}

// ParseRefresh processes CLI arguments, returning a Refresh value, a closer function, and errors.
// If errors are encountered, a Refresh value is still returned representing
// the best effort interpretation of the arguments.
func ParseRefresh(args []string) (*Refresh, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	refresh := &Refresh{
		State:     &State{},
		Operation: &Operation{},
		Vars:      &Vars{},
	}

	cmdFlags := extendedFlagSet("refresh", refresh.State, refresh.Operation, refresh.Vars)
	refresh.ViewOptions.AddFlags(cmdFlags, true)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	args = cmdFlags.Args()
	if len(args) > 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Too many command line arguments",
			"Expected at most one positional argument.",
		))
	}

	diags = diags.Append(refresh.Operation.Parse())
	closer, moreDiags := refresh.ViewOptions.Parse()
	diags = diags.Append(moreDiags)

	return refresh, closer, diags
}
