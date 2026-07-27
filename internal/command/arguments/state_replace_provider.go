// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// StateReplaceProvider represents the command-line arguments for the 'state replace-provider' command.
type StateReplaceProvider struct {
	// RawSrcAddr represents a provider address that is requested by the user to be moved
	RawSrcAddr string
	// RawDestAddr represents a provider address that is requested by the user to be used to move the
	// provider into
	RawDestAddr string
	// AutoApprove is an option that the user can configure to skip the confirmation step of the replacement
	// process.
	AutoApprove bool

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars, Backend and State are the common extended flags
	Vars    *Vars
	Backend *Backend
	State   *State
}

// BindStateReplaceProvider registers CLI arguments, returning a StateReplaceProvider value and it's corresponding hooks.
func BindStateReplaceProvider(cli *CommandLine) *StateReplaceProvider {
	var ret StateReplaceProvider

	ret.ViewOptions.bind(cli, false)

	ret.Vars = &Vars{}
	ret.Vars.bind(cli)

	ret.Backend = &Backend{}
	ret.Backend.bindIgnoreRemoteVersionFlag(cli)

	ret.State = &State{}
	// StateFlagBackup omitted here to be added later with a different default value
	ret.State.bind(cli, stateFlagLock|stateFlagStateIn)
	ret.State.bindBackupFlag(cli, "-")

	cli.BoolVar(&ret.AutoApprove, "auto-approve", false, "Skip interactive approval.")

	cli.ArgHelp = "Exactly two arguments expected"
	cli.PositionalArg(&ret.RawSrcAddr, "src addr", false)
	cli.PositionalArg(&ret.RawDestAddr, "dest addr", false)

	cli.Hook(Hook{Pre: func() tfdiags.Diagnostics {
		// In OpenTofu, there is no way to run a command with `-json` flag and allow asking for user input in the same time.
		// Therefore, the JSON view can used only when the `-auto-approve` is provided too.
		if ret.ViewOptions.ViewType == ViewJSON && !ret.AutoApprove {
			return tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid usage",
				"OpenTofu cannot ask user input when `-json` flag is used. Therefore, `-auto-approve` is required too",
			))
		}
		return nil
	}})

	return &ret
}

// ParseReplaceProvider processes CLI arguments, returning a StateReplaceProvider value, a closer function, and errors.
// If errors are encountered, a StateReplaceProvider value is still returned representing
// the best effort interpretation of the arguments.
func ParseReplaceProvider(args []string) (*StateReplaceProvider, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindStateReplaceProvider(cli)
	closer, diags := cli.Stdlib("state replace-provider", args)
	return ret, closer, diags
}
