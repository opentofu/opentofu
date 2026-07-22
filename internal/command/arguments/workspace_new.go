// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type WorkspaceNew struct {
	// Workspace represents the name of the workspace that the user wants to be selected.
	WorkspaceName string

	// ViewOptions contains the options that allows the user to configure different types of outputs
	// from the current command.
	ViewOptions ViewOptions

	// Vars and State are the common extended flags
	Vars  *Vars
	State *State
}

// BindWorkspaceNew registers CLI arguments, returning a WorkspaceNew value and it's corresponding hooks.
func BindWorkspaceNew(flags Flags) (*WorkspaceNew, Hooks) {
	var ret WorkspaceNew
	var hooks Hooks

	ret.ViewOptions.bind(flags, false)
	hooks = append(hooks, ret.ViewOptions.ParseHook())

	ret.Vars = &Vars{}
	ret.Vars.bind(flags)

	ret.State = &State{}
	ret.State.bind(flags, stateFlagLock|stateFlagStateIn)

	return &ret, hooks
}

func ParseWorkspaceNew(args []string) (*WorkspaceNew, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	ret, hooks := BindWorkspaceNew(flags)

	cmdFlags := defaultFlagSet("workspace new", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	// TODO positional args
	args = cmdFlags.Args()
	if len(args) != 1 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid arguments list",
			"Expected a single argument: NAME.",
		))
	} else {
		ret.WorkspaceName = args[0]
	}

	return ret, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
