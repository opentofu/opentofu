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
func BindStateReplaceProvider(flags Flags) (*StateReplaceProvider, Hooks) {
	var ret StateReplaceProvider
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

	flags.BoolVar(&ret.AutoApprove, "auto-approve", false, "Skip interactive approval.")

	hooks = append(hooks, Hook{Pre: func() tfdiags.Diagnostics {
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

	return &ret, hooks
}

// ParseReplaceProvider processes CLI arguments, returning a StateReplaceProvider value, a closer function, and errors.
// If errors are encountered, a StateReplaceProvider value is still returned representing
// the best effort interpretation of the arguments.
func ParseReplaceProvider(args []string) (*StateReplaceProvider, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	ret, hooks := BindStateReplaceProvider(flags)

	cmdFlags := defaultFlagSet("state replace-provider", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error parsing command-line flags",
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
