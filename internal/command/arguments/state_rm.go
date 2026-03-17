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
	//  BackupPath can be used by the user to configure where to save the backup file of the state file.
	BackupPath string
	// StatePath represents the path of the state to be used for the remove operation.
	StatePath string

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars and Backend are the common extended flags
	Vars    *Vars
	Backend Backend
}

// ParseStateRm processes CLI arguments, returning a StateRm value, a closer function, and errors.
// If errors are encountered, a StateRm value is still returned representing
// the best effort interpretation of the arguments.
func ParseStateRm(args []string) (*StateRm, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret := &StateRm{
		Vars: &Vars{},
	}
	cmdFlags := extendedFlagSet("state rm", nil, nil, ret.Vars)
	ret.Backend.AddIgnoreRemoteVersionFlag(cmdFlags)
	ret.Backend.AddStateFlags(cmdFlags)
	cmdFlags.BoolVar(&ret.DryRun, "dry-run", false, "dry run")
	// NOTE: because the `-backup` flag needs a different default value than usual,
	// we cannot use the [State] flags extension to register and parse these.
	// Therefore, we need to have these redefined here.
	// TODO meta-refactor: we might want to have a separate function on the [arguments.State] to register flags,
	//  where default value can be provided by the caller. This way we could still use those flags instead of redefining
	//  everything again.
	cmdFlags.StringVar(&ret.BackupPath, "backup", "-", "backup-path")
	cmdFlags.StringVar(&ret.StatePath, "state", "", "state-path")

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
