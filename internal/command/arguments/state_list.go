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

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars and State are the common extended flags
	Vars  *Vars
	State *State
}

// BindStateList registers CLI arguments, returning a StateList value and it's corresponding hooks.
func BindStateList(flags Flags) (*StateList, Hooks) {
	var ret StateList
	var hooks Hooks

	ret.ViewOptions.bind(flags, false)
	hooks = append(hooks, ret.ViewOptions.ParseHook())

	ret.Vars = &Vars{}
	ret.Vars.bind(flags)

	ret.State = &State{}
	ret.State.bind(flags, stateFlagStateIn)

	flags.StringVar(&ret.LookupId, "id", "", `Filters the results to include only instances whose resource types have an attribute named "id" whose value equals the given id string.`)

	return &ret, hooks
}

// ParseStateList processes CLI arguments, returning a StateList value, a closer function, and errors.
// If errors are encountered, a StateList value is still returned representing
// the best effort interpretation of the arguments.
func ParseStateList(args []string) (*StateList, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	ret, hooks := BindStateList(flags)

	cmdFlags := defaultFlagSet("state list", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	// TODO positional arguments
	ret.InstancesRawAddr = cmdFlags.Args()

	return ret, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
