// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// StatePush represents the command-line arguments for the 'state push' command.
type StatePush struct {
	// StateSrc represents the source of the state that wants to be pushed.
	// This can be a file name/file path, or it can be "-" when the state should be read from [os.Stdin].
	StateSrc string
	// Force will try to forcefully push the state remotely. This will happen only if the backend supports it.
	Force bool
	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars and Backend are the common extended flags
	Vars    *Vars
	Backend Backend
}

// ParseStatePush processes CLI arguments, returning a StatePush value, a closer function, and errors.
// If errors are encountered, a StatePush value is still returned representing
// the best effort interpretation of the arguments.
func ParseStatePush(args []string) (*StatePush, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret := &StatePush{
		Vars: &Vars{},
	}
	cmdFlags := extendedFlagSet("state push", nil, ret.Vars)
	ret.Backend.AddIgnoreRemoteVersionFlag(cmdFlags)
	ret.Backend.AddStateFlags(cmdFlags)
	cmdFlags.BoolVar(&ret.Force, "force", false, "")
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
		ret.StateSrc = args[0]
	}

	closer, moreDiags := ret.ViewOptions.Parse()
	diags = diags.Append(moreDiags)

	return ret, closer, diags
}
