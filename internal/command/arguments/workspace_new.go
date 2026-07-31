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
func BindWorkspaceNew(cli *CommandLine) *WorkspaceNew {
	var ret WorkspaceNew

	ret.ViewOptions.bind(cli, false)

	ret.Vars = &Vars{}
	ret.Vars.bind(cli)

	ret.State = BindState(cli, stateFlagLock|stateFlagStateIn)

	cli.ArgHelp = "Expected a single argument: NAME."
	cli.PositionalArg(&ret.WorkspaceName, "NAME", false)

	return &ret
}

func ParseWorkspaceNew(args []string) (*WorkspaceNew, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindWorkspaceNew(cli)
	closer, diags := cli.Stdlib("workspace new", args)
	return ret, closer, diags
}
