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
	// View represents the global view options
	View *View

	// Vars, Backend and State are the common extended flags
	Vars    *Vars
	Backend *Backend
	State   *State
}

// BindStatePush registers CLI arguments, returning a StatePush value and it's corresponding hooks.
func BindStatePush(cli *CommandLine) *StatePush {
	ret := StatePush{
		View:    BindView(cli, viewFlagNoInput),
		Vars:    BindVars(cli),
		Backend: BindBackend(cli),
		State:   BindState(cli, stateFlagLock),
	}

	cli.BoolVar(&ret.Force, "force", false, "Write the state even if lineages don't match or the remote serial is higher.")

	cli.PositionalArg(&ret.StateSrc, "PATH", false)

	return &ret
}

// ParseStatePush processes CLI arguments, returning a StatePush value, a closer function, and errors.
// If errors are encountered, a StatePush value is still returned representing
// the best effort interpretation of the arguments.
func ParseStatePush(args []string) (*StatePush, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindStatePush(cli)
	closer, diags := cli.parseWithHooks("state push", args)
	return ret, closer, diags
}
