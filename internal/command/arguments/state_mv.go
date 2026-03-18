// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// StateMv represents the command-line arguments for the 'state mv' command.
type StateMv struct {
	// RawSrcAddr represents a resources address that is requested by the user to be moved
	RawSrcAddr string
	// RawDestAddr represents a resources address that is requested by the user to be used to move the
	// resource into
	RawDestAddr string
	// DryRun just validates that the arguments provided are valid and will output the possible outcome.
	// When running in this mode, the state will suffer no change.
	DryRun bool
	// BackupPathOut and BackupPath can be used by the user to configure where to save the backup file of the state file.
	BackupPathOut string
	BackupPath    string
	// StatePath represents the path of the state to be used for the moving operation.
	StatePath string
	// StateOutPath represents the path where OpenTofu should save the new state.
	StateOutPath string

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars and Backend are the common extended flags
	Vars    *Vars
	Backend Backend
}

// ParseStateMv processes CLI arguments, returning a StateMv value, a closer function, and errors.
// If errors are encountered, a StateMv value is still returned representing
// the best effort interpretation of the arguments.
func ParseStateMv(args []string) (*StateMv, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret := &StateMv{
		Vars: &Vars{},
	}

	cmdFlags := extendedFlagSet("state mv", nil, nil, ret.Vars)
	ret.Backend.AddIgnoreRemoteVersionFlag(cmdFlags)
	ret.Backend.AddStateFlags(cmdFlags)
	cmdFlags.BoolVar(&ret.DryRun, "dry-run", false, "dry run")
	// NOTE: because the `-backup` and `-backup-out` flags need a different default value than usual,
	// we cannot use the [State] flags extension to register and parse these.
	// Therefore, we need to have these redefined here.
	// TODO meta-refactor: we might want to have a separate function on the [arguments.State] to register flags,
	//  where default value can be provided by the caller. This way we could still use those flags instead of redefining
	//  everything again.
	cmdFlags.StringVar(&ret.BackupPath, "backup", "-", "backup-path")
	cmdFlags.StringVar(&ret.BackupPathOut, "backup-out", "-", "backup")
	cmdFlags.StringVar(&ret.StatePath, "state", "", "state-path")
	cmdFlags.StringVar(&ret.StateOutPath, "state-out", "", "state-path")

	ret.ViewOptions.AddFlags(cmdFlags, false)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
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

	return ret, closer, diags
}
