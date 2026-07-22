// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Refresh represents the command-line arguments for the apply command.
type Refresh struct {
	// State, Operation, and Vars are the common extended flags
	State     *State
	Operation *Operation
	Vars      *Vars

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions
}

// BindRefresh registers CLI arguments, returning a Refresh value and it's corresponding hooks.
func BindRefresh(flags Flags) (*Refresh, Hooks) {
	var refresh Refresh
	var hooks Hooks

	refresh.ViewOptions.bind(flags, true)
	hooks = append(hooks, refresh.ViewOptions.ParseHook())

	refresh.Vars = &Vars{}
	refresh.Vars.bind(flags)

	refresh.Operation = &Operation{}
	refresh.Operation.bind(flags)
	hooks = append(hooks, Hook{Pre: refresh.Operation.Parse})

	refresh.State = &State{}
	refresh.State.bind(flags, stateFlagAll)

	return &refresh, hooks
}

// ParseRefresh processes CLI arguments, returning a Refresh value, a closer function, and errors.
// If errors are encountered, a Refresh value is still returned representing
// the best effort interpretation of the arguments.
func ParseRefresh(args []string) (*Refresh, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	refresh, hooks := BindRefresh(flags)

	cmdFlags := defaultFlagSet("refresh", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	args = cmdFlags.Args()
	if len(args) > 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Too many command line arguments",
			"Expected at most one positional argument.",
		))
	}

	return refresh, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
