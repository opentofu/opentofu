// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// StateRm represents the command-line arguments for the 'state rm' command.
type StateRm struct {
	// TargetAddrs represents the raw resource addresses to be removed from the state
	TargetAddrs []string
	// DryRun just validates that the arguments provided are valid and will output the possible outcome.
	// When running in this mode, the state will suffer no change.
	DryRun bool

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars, Backend and State are the common extended flags
	Vars    *Vars
	Backend *Backend
	State   *State
}

// ParseStateRm processes CLI arguments, returning a StateRm value, a closer function, and errors.
// If errors are encountered, a StateRm value is still returned representing
// the best effort interpretation of the arguments.
func ParseStateRm(args []string) (*StateRm, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret := &StateRm{
		Vars:    &Vars{},
		Backend: &Backend{},
		State:   &State{},
	}
	cmdFlags := extendedFlagSet("state rm", nil, ret.Vars)
	ret.Backend.AddIgnoreRemoteVersionFlag(cmdFlags)
	ret.State.AddFlags(cmdFlags, true, true, false, false)
	ret.State.AddBackupFlag(cmdFlags, "-")
	cmdFlags.BoolVar(&ret.DryRun, "dry-run", false, "dry run")

	ret.ViewOptions.AddFlags(cmdFlags, false)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	args = cmdFlags.Args()
	if len(args) == 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid number of arguments",
			"At least one address is required",
		))
	} else {
		ret.TargetAddrs = args
	}

	closer, moreDiags := ret.ViewOptions.Parse()
	diags = diags.Append(moreDiags)

	return ret, closer, diags
}
