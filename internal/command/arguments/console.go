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
	// View represents the global view options
	View *View
	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
	// State is used for the state related flags
	State *State
}

// BindConsole registers CLI arguments, returning a Console value and it's corresponding hooks.
func BindConsole(cli *CommandLine) *Console {
	console := Console{
		View:  BindView(cli, viewFlagAll),
		Vars:  BindVars(cli),
		State: BindState(cli, stateFlagLock|stateFlagStateIn),
	}

	cli.PreHook(func() tfdiags.Diagnostics {
		if console.State.StatePath == "" {
			console.State.StatePath = DefaultStateFilename
		}
		// If the user provided the -json flag, we don't allow it since the UX is just poor in this case.
		// We allow only the streaming of the evaluated values in a json file, by using the `-json-into` flag.
		if console.View.ViewType == ViewJSON {
			// Revert the view type to be able to print the diagnostic properly
			console.View.ViewType = ViewHuman
			return tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Output only in json is not allowed",
				"In case you want to stream the output of the console into json, use the \"-json-into\" instead.",
			))
		}
		return nil
	})

	return &console
}

// ParseConsole processes CLI arguments, returning a Console value, a closer function, and errors.
// If errors are encountered, a Console value is still returned representing
// the best effort interpretation of the arguments.
func ParseConsole(args []string) (*Console, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	console := BindConsole(cli)
	closer, diags := cli.parseWithHooks("console", args)
	return console, closer, diags
}
