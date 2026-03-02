// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Untaint represents the command-line arguments for the untaint command.
type Untaint struct {
	// TargetAddress is the resource address that is requested to be marked as tainted.
	TargetAddress addrs.AbsResourceInstance
	// AllowMissing can be set to "true" to write a warning instead of an error and to return exit code 0
	// when the TargetAddress points to a missing resource.
	AllowMissing bool

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars

	// TODO meta-refactor: Backend and State have overlapping flags. Those need to be unified and
	//  have a single point of registration of those flags

	// State is used for the state related flags
	State *State
	// Backend is used strictly for the ignore remote version flag
	Backend *Backend
}

// ParseUntaint processes CLI arguments, returning a Untaint value, a closer function, and errors.
// If errors are encountered, a Untaint value is still returned representing
// the best effort interpretation of the arguments.
func ParseUntaint(args []string) (*Untaint, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	arguments := &Untaint{
		Vars:    &Vars{},
		State:   &State{},
		Backend: &Backend{},
	}

	cmdFlags := extendedFlagSet("untaint", arguments.State, nil, arguments.Vars)
	cmdFlags.BoolVar(&arguments.AllowMissing, "allow-missing", false, "allow missing")
	arguments.Backend.AddIgnoreRemoteVersionFlag(cmdFlags)
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
	args = cmdFlags.Args()
	if len(args) != 1 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid arguments",
			"The untaint command expects exactly one argument.",
		))
	} else {
		addr, addrDiags := addrs.ParseAbsResourceInstanceStr(args[0])
		diags = diags.Append(addrDiags)
		arguments.TargetAddress = addr
	}

	return arguments, closer, diags
}
