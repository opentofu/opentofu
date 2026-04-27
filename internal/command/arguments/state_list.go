// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// StateList represents the command-line arguments for the 'state list' command.
type StateList struct {
	// LookupId restricts output to paths with a resource having the specified ID.
	LookupId string
	// InstancesRawAddr is a list of raw addresses of the resources that are requested
	// to be listed.
	InstancesRawAddr []string

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars and State are the common extended flags
	Vars  *Vars
	State *State
}

// ParseStateList processes CLI arguments, returning a StateList value, a closer function, and errors.
// If errors are encountered, a StateList value is still returned representing
// the best effort interpretation of the arguments.
func ParseStateList(args []string) (*StateList, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret := &StateList{
		Vars:  &Vars{},
		State: &State{}, // Initialised like this because we don't want to have the lock enabled by default
	}

	cmdFlags := extendedFlagSet("state list", nil, ret.Vars)
	cmdFlags.StringVar(&ret.LookupId, "id", "", "Restrict output to paths with a resource having the specified ID.")
	ret.State.AddFlags(cmdFlags, false, true, false, false)
	ret.ViewOptions.AddFlags(cmdFlags, false)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	ret.InstancesRawAddr = cmdFlags.Args()

	closer, moreDiags := ret.ViewOptions.Parse()
	diags = diags.Append(moreDiags)

	return ret, closer, diags
}
