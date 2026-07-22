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

// BindStateRm registers CLI arguments, returning a StateRm value and it's corresponding hooks.
func BindStateRm(flags Flags) (*StateRm, Hooks) {
	var ret StateRm
	var hooks Hooks

	ret.ViewOptions.bind(flags, false)
	hooks = append(hooks, ret.ViewOptions.ParseHook())

	ret.Vars = &Vars{}
	ret.Vars.bind(flags)

	ret.Backend = &Backend{}
	ret.Backend.bindIgnoreRemoteVersionFlag(flags)

	ret.State = &State{}
	// StateFlagBackup omitted here to be added later with a different default value
	ret.State.bind(flags, stateFlagLock|stateFlagStateIn)
	ret.State.bindBackupFlag(flags, "-")

	flags.BoolVar(&ret.DryRun, "dry-run", false, "If set, prints out what would've been removed but doesn't actually remove anything.")

	return &ret, hooks
}

// ParseStateRm processes CLI arguments, returning a StateRm value, a closer function, and errors.
// If errors are encountered, a StateRm value is still returned representing
// the best effort interpretation of the arguments.
func ParseStateRm(args []string) (*StateRm, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	ret, hooks := BindStateRm(flags)

	cmdFlags := defaultFlagSet("state rm", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	// TODO positional arguments
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

	return ret, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
