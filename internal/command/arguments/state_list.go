// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// StateList represents the command-line arguments for the 'state list' command.
type StateList struct {
	// LookupId restricts output to paths with a resource having the specified ID.
	LookupId string
	// InstancesRawAddr is a list of raw addresses of the resources that are requested
	// to be listed.
	InstancesRawAddr []string

	// View represents the global view options
	View *View

	// Vars and State are the common extended flags
	Vars  *Vars
	State *State
}

// BindStateList registers CLI arguments, returning a StateList value and it's corresponding hooks.
func BindStateList(cli *CommandLine) *StateList {
	ret := StateList{
		View:  BindView(cli, viewFlagNoInput),
		Vars:  BindVars(cli),
		State: BindState(cli, stateFlagStateIn),
	}

	cli.StringVar(&ret.LookupId, "id", "", `Filters the results to include only instances whose resource types have an attribute named "id" whose value equals the given id string.`).SetDisplay("=ID")

	cli.VariadicArg(&ret.InstancesRawAddr, "address")

	return &ret
}

// ParseStateList processes CLI arguments, returning a StateList value, a closer function, and errors.
// If errors are encountered, a StateList value is still returned representing
// the best effort interpretation of the arguments.
func ParseStateList(args []string) (*StateList, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindStateList(cli)
	closer, diags := cli.parseWithHooks("state list", args)
	return ret, closer, diags
}
