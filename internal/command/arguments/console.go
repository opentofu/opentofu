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
	// StatePath is the path to the state file to use for the console session.
	StatePath string

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars

	// Backend is used here to register and parse the flags for state locking
	Backend Backend
}

// ParseConsole processes CLI arguments, returning a Console value, a closer function, and errors.
// If errors are encountered, a Console value is still returned representing
// the best effort interpretation of the arguments.
func ParseConsole(args []string) (*Console, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	console := &Console{
		Vars: &Vars{},
	}

	cmdFlags := extendedFlagSet("console", nil, nil, console.Vars)
	console.Backend.AddStateFlags(cmdFlags)
	cmdFlags.StringVar(&console.StatePath, "state", DefaultStateFilename, "path")

	console.ViewOptions.AddFlags(cmdFlags, true)

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

	closer, moreDiags := console.ViewOptions.Parse()
	diags = diags.Append(moreDiags)
	// If the user provided the -json flag, we don't allow it since the UX is just poor in this case.
	// We allow only the streaming of the evaluated values in a json file, by using the `-json-into` flag.
	if console.ViewOptions.ViewType == ViewJSON {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Output only in json is not allowed",
			"In case you want to stream the output of the console into json, use the \"-json-into\" instead.",
		))
		// Revert the view type to be able to print the diagnostic properly
		console.ViewOptions.ViewType = ViewHuman
	}

	return console, closer, diags
}
