// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type WorkspaceDelete struct {
	// WorkspaceName represents the name of the workspace that the user wants to be selected.
	WorkspaceName string

	// Force allows the user to forcefully delete a workspace removing the still existing resources
	// from the OpenTofu's management.
	Force bool

	// ViewOptions contains the options that allows the user to configure different types of outputs
	// from the current command.
	ViewOptions ViewOptions

	// Vars and State are the common extended flags
	Vars  *Vars
	State *State
}

// BindWorkspaceDelete registers CLI arguments, returning a WorkspaceDelete value and it's corresponding hooks.
func BindWorkspaceDelete(flags Flags) (*WorkspaceDelete, Hooks) {
	var ret WorkspaceDelete
	var hooks Hooks

	ret.ViewOptions.bind(flags, false)
	hooks = append(hooks, ret.ViewOptions.ParseHook())

	ret.Vars = &Vars{}
	ret.Vars.bind(flags)

	ret.State = &State{}
	ret.State.bind(flags, stateFlagLock)

	flags.BoolVar(&ret.Force, "force", false, "Remove a workspace even if it is managing resources. OpenTofu can no longer track or manage the workspace's infrastructure.")

	return &ret, hooks
}

func ParseWorkspaceDelete(args []string) (*WorkspaceDelete, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	ret, hooks := BindWorkspaceDelete(flags)

	cmdFlags := defaultFlagSet("workspace delete", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	// TODO positional arguments
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
