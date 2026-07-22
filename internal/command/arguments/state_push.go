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

	// Vars, Backend and State are the common extended flags
	Vars    *Vars
	Backend *Backend
	State   *State
}

// BindStatePush registers CLI arguments, returning a StatePush value and it's corresponding hooks.
func BindStatePush(flags Flags) (*StatePush, Hooks) {
	var ret StatePush
	var hooks Hooks

	ret.ViewOptions.bind(flags, false)
	hooks = append(hooks, ret.ViewOptions.ParseHook())

	ret.Vars = &Vars{}
	ret.Vars.bind(flags)

	ret.Backend = &Backend{}
	ret.Backend.bindIgnoreRemoteVersionFlag(flags)

	ret.State = &State{}
	ret.State.bind(flags, stateFlagLock)

	flags.BoolVar(&ret.Force, "force", false, "Write the state even if lineages don't match or the remote serial is higher.")

	return &ret, hooks
}

// ParseStatePush processes CLI arguments, returning a StatePush value, a closer function, and errors.
// If errors are encountered, a StatePush value is still returned representing
// the best effort interpretation of the arguments.
func ParseStatePush(args []string) (*StatePush, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	ret, hooks := BindStatePush(flags)

	cmdFlags := defaultFlagSet("state push", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	// TODO positional arguments
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

	return ret, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
