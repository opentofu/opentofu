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
	// BackupPathOut can be used by the user to configure where to save the backup file of the state file.
	BackupPathOut string

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars, Backend and State are the common extended flags
	Vars    *Vars
	Backend *Backend
	State   *State
}

// BindStateMv registers CLI arguments, returning a StateMv value and it's corresponding hooks.
func BindStateMv(flags Flags) (*StateMv, Hooks) {
	var ret StateMv
	var hooks Hooks

	ret.ViewOptions.bind(flags, false)
	hooks = append(hooks, ret.ViewOptions.ParseHook())

	ret.Vars = &Vars{}
	ret.Vars.bind(flags)

	ret.Backend = &Backend{}
	ret.Backend.bindIgnoreRemoteVersionFlag(flags)

	ret.State = &State{}
	ret.State.bind(flags, stateFlagLock|stateFlagStateIn|stateFlagStateOut)
	ret.State.bindBackupFlag(flags, "-")
	// StateFlagBackup omitted here to be added later with a different default value

	flags.BoolVar(&ret.DryRun, "dry-run", false, "dry run")
	flags.StringVar(&ret.BackupPathOut, "backup-out", "-", "backup")

	return &ret, hooks
}

// ParseStateMv processes CLI arguments, returning a StateMv value, a closer function, and errors.
// If errors are encountered, a StateMv value is still returned representing
// the best effort interpretation of the arguments.
func ParseStateMv(args []string) (*StateMv, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	ret, hooks := BindStateMv(flags)

	cmdFlags := defaultFlagSet("state mv", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	// TODO positional arguments
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

	return ret, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
