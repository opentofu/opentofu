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
	// ShowSensitive forces the show command to print also the sensitive values of the targeted resource.
	// This applies only to the [views.StateHuman] since the [views.StateJSON] shows the sensitive values
	// all the time.
	ShowSensitive bool

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars and State are the common extended flags
	Vars  *Vars
	State *State
}

// BindStateShow registers CLI arguments, returning a StateShow value and it's corresponding hooks.
func BindStateShow(cli *CommandLine) *StateShow {
	var ret StateShow

	ret.ViewOptions.bind(cli, false)

	ret.Vars = &Vars{}
	ret.Vars.bind(cli)

	ret.State = BindState(cli, stateFlagStateIn)

	cli.BoolVar(&ret.ShowSensitive, "show-sensitive", false, "If specified, sensitive values will be displayed.")

	cli.PositionalArg(&ret.TargetRawAddr, "ADDRESS", false)

	return &ret
}

// ParseStateShow processes CLI arguments, returning a StateShow value, a closer function, and errors.
// If errors are encountered, a StateShow value is still returned representing
// the best effort interpretation of the arguments.
func ParseStateShow(args []string) (*StateShow, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindStateShow(cli)
	closer, diags := cli.Stdlib("state show", args)
	return ret, closer, diags
}
