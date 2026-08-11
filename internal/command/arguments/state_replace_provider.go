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

	// View represents the global view options
	View *View

	// Vars, Backend and State are the common extended flags
	Vars    *Vars
	Backend *Backend
	State   *State
}

// BindStateReplaceProvider registers CLI arguments, returning a StateReplaceProvider value and it's corresponding hooks.
func BindStateReplaceProvider(cli *CommandLine) *StateReplaceProvider {
	ret := StateReplaceProvider{
		View:    BindView(cli, viewFlagNoInput),
		Vars:    BindVars(cli),
		Backend: BindBackend(cli),
		State:   BindState(cli, stateFlagLock|stateFlagStateIn|stateFlagBackup),
	}

	cli.BoolVar(&ret.AutoApprove, "auto-approve", false, "Skip interactive approval.")

	cli.PositionalArg(&ret.RawSrcAddr, "FROM_PROVIDER_FQN", false)
	cli.PositionalArg(&ret.RawDestAddr, "TO_PROVIDER_FQN", false)

	cli.PreHook(func() tfdiags.Diagnostics {
		if ret.State.BackupPath == "" {
			ret.State.BackupPath = "-"
		}
		// In OpenTofu, there is no way to run a command with `-json` flag and allow asking for user input in the same time.
		// Therefore, the JSON view can used only when the `-auto-approve` is provided too.
		if ret.View.ViewType == ViewJSON && !ret.AutoApprove {
			return tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid usage",
				"OpenTofu cannot ask user input when `-json` flag is used. Therefore, `-auto-approve` is required too",
			))
		}
		return nil
	})

	cli.FlagGroups = []FlagGroup{{
		ID:     "",
		Title:  "Options:",
		Suffix: `-state, state-out, and -backup are legacy options supported for the local backend only. For more information, see the local backend's documentation.`,
	}}

	return &ret
}

// ParseReplaceProvider processes CLI arguments, returning a StateReplaceProvider value, a closer function, and errors.
// If errors are encountered, a StateReplaceProvider value is still returned representing
// the best effort interpretation of the arguments.
func ParseReplaceProvider(args []string) (*StateReplaceProvider, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindStateReplaceProvider(cli)
	closer, diags := cli.parseWithHooks("state replace-provider", args)
	return ret, closer, diags
}
