// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// StateShow represents the command-line arguments for the 'state show' command.
type StateShow struct {
	// TargetRawAddr represents the raw resource address of the resource requested to have the state shown for.
	TargetRawAddr string

	// View represents the global view options
	View *View

	// Vars and State are the common extended flags
	Vars  *Vars
	State *State
}

// BindStateShow registers CLI arguments, returning a StateShow value and it's corresponding hooks.
func BindStateShow(cli *CommandLine) *StateShow {
	ret := StateShow{
		View:  BindView(cli, viewFlagNoInput|viewFlagSensitive),
		Vars:  BindVars(cli),
		State: BindState(cli, stateFlagStateIn),
	}

	cli.PositionalArg(&ret.TargetRawAddr, "ADDRESS", false)

	return &ret
}

// ParseStateShow processes CLI arguments, returning a StateShow value, a closer function, and errors.
// If errors are encountered, a StateShow value is still returned representing
// the best effort interpretation of the arguments.
func ParseStateShow(args []string) (*StateShow, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindStateShow(cli)
	closer, diags := cli.parseWithHooks("state show", args)
	return ret, closer, diags
}
