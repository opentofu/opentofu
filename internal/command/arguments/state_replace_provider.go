// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// StateReplaceProvider represents the command-line arguments for the 'state replace-provider' command.
type StateReplaceProvider struct {
	// RawSrcAddr represents a provider address that is requested by the user to be moved
	RawSrcAddr string
	// RawDestAddr represents a provider address that is requested by the user to be used to move the
	// provider into
	RawDestAddr string
	// AutoApprove is an option that the user can configure to skip the confirmation step of the replacement
	// process.
	AutoApprove bool

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars, Backend and State are the common extended flags
	Vars    *Vars
	Backend Backend
	State   *State
}

// ParseReplaceProvider processes CLI arguments, returning a StateReplaceProvider value, a closer function, and errors.
// If errors are encountered, a StateReplaceProvider value is still returned representing
// the best effort interpretation of the arguments.
func ParseReplaceProvider(args []string) (*StateReplaceProvider, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret := &StateReplaceProvider{
		Vars:  &Vars{},
		State: &State{},
	}

	cmdFlags := extendedFlagSet("state replace-provider", nil, ret.Vars)
	cmdFlags.BoolVar(&ret.AutoApprove, "auto-approve", false, "skip interactive approval of replacements")
	ret.Backend.AddIgnoreRemoteVersionFlag(cmdFlags)
	ret.State.AddFlags(cmdFlags, true, true, false, false)
	ret.State.AddBackupFlag(cmdFlags, "-")
	ret.ViewOptions.AddFlags(cmdFlags, false)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error parsing command-line flags",
			err.Error(),
		))
	}

	args = cmdFlags.Args()
	if len(args) != 2 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid number of arguments",
			"Exactly two arguments expected",
		))
	} else {
		ret.RawSrcAddr = args[0]
		ret.RawDestAddr = args[1]
	}
	closer, moreDiags := ret.ViewOptions.Parse()
	diags = diags.Append(moreDiags)
	// In OpenTofu, there is no way to run a command with `-json` flag and allow asking for user input in the same time.
	// Therefore, the JSON view can used only when the `-auto-approve` is provided too.
	if ret.ViewOptions.ViewType == ViewJSON && !ret.AutoApprove {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid usage",
			"OpenTofu cannot ask user input when `-json` flag is used. Therefore, `-auto-approve` is required too",
		))
	}

	return ret, closer, diags
}
