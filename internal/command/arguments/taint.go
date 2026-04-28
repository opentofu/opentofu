// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Taint represents the command-line arguments for the taint and untaint commands.
type Taint struct {
	// TargetAddress is the resource address that is requested to be marked as tainted.
	TargetAddress addrs.AbsResourceInstance
	// AllowMissing can be set to "true" to write a warning instead of an error and to return exit code 0
	// when the TargetAddress points to a missing resource.
	AllowMissing bool

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars

	// State is used for the state related flags
	State *State
	// Backend is used strictly for the ignore remote version flag
	Backend *Backend
}

// ParseTaint processes CLI arguments, returning a Taint value, a closer function, and errors.
// If errors are encountered, a Taint value is still returned representing
// the best effort interpretation of the arguments.
func ParseTaint(isTaint bool, args []string) (*Taint, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	arguments := &Taint{
		Vars:    &Vars{},
		State:   &State{},
		Backend: &Backend{},
	}
	cmd := "taint"
	if !isTaint {
		cmd = "untaint"
	}
	cmdFlags := extendedFlagSet(cmd, nil, arguments.Vars)
	arguments.State.AddFlags(cmdFlags, true, true, true, true)
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
			fmt.Sprintf("The %s command expects exactly one argument.", cmd),
		))
	} else {
		addr, addrDiags := addrs.ParseAbsResourceInstanceStr(args[0])
		diags = diags.Append(addrDiags)
		arguments.TargetAddress = addr
		if !diags.HasErrors() {
			if addr.Resource.Resource.Mode != addrs.ManagedResourceMode && isTaint {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Invalid resource address",
					fmt.Sprintf("Resource instance %s cannot be tainted", addr),
				))
			}
		}

	}

	return arguments, closer, diags
}
