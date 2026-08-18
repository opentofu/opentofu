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

	// View represents the global view options
	View *View

	// Vars, Backend and State are the common extended flags
	Vars    *Vars
	Backend *Backend
	State   *State
}

// BindStateRm registers CLI arguments, returning a StateRm value and it's corresponding hooks.
func BindStateRm(cli *CommandLine) *StateRm {
	ret := StateRm{
		View:    BindView(cli, viewFlagNoInput),
		Vars:    BindVars(cli),
		Backend: BindBackend(cli),
		State:   BindState(cli, stateFlagLock|stateFlagStateIn|stateFlagBackup),
	}

	cli.BoolVar(&ret.DryRun, "dry-run", false, "If set, prints out what would've been removed but doesn't actually remove anything.")

	cli.VariadicArg(&ret.TargetAddrs, "ADDRESS")

	cli.PreHook(func() tfdiags.Diagnostics {
		if ret.State.BackupPath == "" {
			ret.State.BackupPath = "-"
		}

		if len(ret.TargetAddrs) == 0 {
			return tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid number of arguments",
				"At least one address is required",
			))
		}
		return nil
	})

	return &ret
}

// ParseStateRm processes CLI arguments, returning a StateRm value, a closer function, and errors.
// If errors are encountered, a StateRm value is still returned representing
// the best effort interpretation of the arguments.
func ParseStateRm(args []string) (*StateRm, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindStateRm(cli)
	closer, diags := cli.parseWithHooks("state rm", args)
	return ret, closer, diags
}
