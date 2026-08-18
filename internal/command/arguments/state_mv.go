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

	// View represents the global view options
	View *View

	// Vars, Backend and State are the common extended flags
	Vars    *Vars
	Backend *Backend
	State   *State
}

// BindStateMv registers CLI arguments, returning a StateMv value and it's corresponding hooks.
func BindStateMv(cli *CommandLine) *StateMv {
	ret := StateMv{
		View:    BindView(cli, viewFlagNoInput),
		Vars:    BindVars(cli),
		Backend: BindBackend(cli),
		State:   BindState(cli, stateFlagAll),
	}

	cli.BoolVar(&ret.DryRun, "dry-run", false, "If set, prints out what would've been moved but doesn't actually move anything.")
	cli.StringVar(&ret.BackupPathOut, "backup-out", "-", "Legacy state backup option").SetHidden(true)

	cli.PositionalArg(&ret.RawSrcAddr, "SOURCE", false)
	cli.PositionalArg(&ret.RawDestAddr, "DESTINATION", false)

	cli.PreHook(func() tfdiags.Diagnostics {
		if ret.State.BackupPath == "" {
			ret.State.BackupPath = "-"
		}
		return nil
	})

	return &ret
}

// ParseStateMv processes CLI arguments, returning a StateMv value, a closer function, and errors.
// If errors are encountered, a StateMv value is still returned representing
// the best effort interpretation of the arguments.
func ParseStateMv(args []string) (*StateMv, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindStateMv(cli)
	closer, diags := cli.parseWithHooks("state mv", args)
	return ret, closer, diags
}
