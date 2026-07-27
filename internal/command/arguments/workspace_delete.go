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
func BindWorkspaceDelete(cli *CommandLine) *WorkspaceDelete {
	var ret WorkspaceDelete

	ret.ViewOptions.bind(cli, false)

	ret.Vars = &Vars{}
	ret.Vars.bind(cli)

	ret.State = &State{}
	ret.State.bind(cli, stateFlagLock)

	cli.BoolVar(&ret.Force, "force", false, "Remove a workspace even if it is managing resources. OpenTofu can no longer track or manage the workspace's infrastructure.")

	cli.ArgHelp = "Expected a single argument: NAME."
	cli.PositionalArg(&ret.WorkspaceName, "NAME", false)

	return &ret
}

func ParseWorkspaceDelete(args []string) (*WorkspaceDelete, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindWorkspaceDelete(cli)
	closer, diags := cli.Stdlib("workspace delete", args)
	return ret, closer, diags
}
