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

	// View represents the global view options
	View *View

	// Vars and State are the common extended flags
	Vars  *Vars
	State *State
}

// BindWorkspaceNew registers CLI arguments, returning a WorkspaceNew value and it's corresponding hooks.
func BindWorkspaceNew(cli *CommandLine) *WorkspaceNew {
	ret := WorkspaceNew{
		View:  BindView(cli, viewFlagNoInput),
		Vars:  BindVars(cli),
		State: BindState(cli, stateFlagLock|stateFlagStateIn),
	}

	cli.ArgHelp = "Expected a single argument: NAME."
	cli.PositionalArg(&ret.WorkspaceName, "NAME", false)

	return &ret
}

func ParseWorkspaceNew(args []string) (*WorkspaceNew, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindWorkspaceNew(cli)
	closer, diags := cli.parseWithHooks("workspace new", args)
	return ret, closer, diags
}
