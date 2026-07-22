// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Console represents the command-line arguments for the console command.
type Console struct {
	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions
	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
	// State is used for the state related flags
	State *State
}

// BindConsole registers CLI arguments, returning a Console value and it's corresponding hooks.
func BindConsole(flags Flags) (*Console, Hooks) {
	var console Console
	var hooks Hooks

	console.Vars = &Vars{}
	console.Vars.bind(flags)

	console.State = &State{}
	console.State.bind(flags, stateFlagLock)
	console.State.bindStateInFlag(flags, DefaultStateFilename)

	console.ViewOptions.bind(flags, true)

	hooks = append(hooks, console.ViewOptions.ParseHook())
	hooks = append(hooks, Hook{Pre: func() tfdiags.Diagnostics {
		// If the user provided the -json flag, we don't allow it since the UX is just poor in this case.
		// We allow only the streaming of the evaluated values in a json file, by using the `-json-into` flag.
		if console.ViewOptions.ViewType == ViewJSON {
			return tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Output only in json is not allowed",
				"In case you want to stream the output of the console into json, use the \"-json-into\" instead.",
			))
			// Revert the view type to be able to print the diagnostic properly
			console.ViewOptions.ViewType = ViewHuman
		}
		return nil
	}})

	return &console, hooks
}

// ParseConsole processes CLI arguments, returning a Console value, a closer function, and errors.
// If errors are encountered, a Console value is still returned representing
// the best effort interpretation of the arguments.
func ParseConsole(args []string) (*Console, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	console, hooks := BindConsole(flags)

	cmdFlags := defaultFlagSet("console", flags)

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

	diags = diags.Append(hooks.Pre())
	return console, func() { hooks.Post() }, diags
}
