// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// StateShow represents the command-line arguments for the 'state show' command.
type StateShow struct {
	// TargetRawAddr represents the raw resource address of the resource requested to have the state shown for.
	TargetRawAddr string
	// ShowSensitive forces the show command to print also the sensitive values of the targeted resource.
	// This applies only to the [views.StateHuman] since the [views.StateJSON] shows the sensitive values
	// all the time.
	ShowSensitive bool

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars and State are the common extended flags
	Vars  *Vars
	State *State
}

// ParseStateShow processes CLI arguments, returning a StateShow value, a closer function, and errors.
// If errors are encountered, a StateShow value is still returned representing
// the best effort interpretation of the arguments.
func ParseStateShow(args []string) (*StateShow, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret := &StateShow{
		Vars:  &Vars{},
		State: &State{},
	}
	cmdFlags := extendedFlagSet("state show", nil, ret.Vars)
	cmdFlags.BoolVar(&ret.ShowSensitive, "show-sensitive", false, "displays sensitive values")
	ret.State.addFlags(cmdFlags, stateFlagStateIn)

	ret.ViewOptions.AddFlags(cmdFlags, false)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	args = cmdFlags.Args()
	if len(args) != 1 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid number of arguments",
			"Exactly one argument expected",
		))
	} else {
		ret.TargetRawAddr = args[0]
	}

	closer, moreDiags := ret.ViewOptions.Parse()
	diags = diags.Append(moreDiags)

	return ret, closer, diags
}
