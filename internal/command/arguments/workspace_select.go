// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type WorkspaceSelect struct {
	// Workspace represents the name of the workspace that the user wants to be selected.
	WorkspaceName string
	// CreateIfMissing is a flag that the user can set to "true" to force the creation of the workspace
	// in case it's missing from the current list of workspaces.
	CreateIfMissing bool

	// View represents the global view options
	View *View

	// Vars holds the information that might be needed to be given through `-var`/`-var-file`.
	Vars *Vars
}

// BindWorkspaceSelect registers CLI arguments, returning a WorkspaceSelect value and it's corresponding hooks.
func BindWorkspaceSelect(cli *CommandLine) *WorkspaceSelect {
	ret := WorkspaceSelect{
		View: BindView(cli, viewFlagNoInput),
		Vars: BindVars(cli),
	}

	cli.BoolVar(&ret.CreateIfMissing, "or-create", false, "Create the OpenTofu workspace if it doesn't exist.")

	cli.ArgHelp = "Expected a single argument: NAME."
	cli.PositionalArg(&ret.WorkspaceName, "NAME", false)

	return &ret
}

func ParseWorkspaceSelect(args []string) (*WorkspaceSelect, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindWorkspaceSelect(cli)
	closer, diags := cli.parseWithHooks("workspace select", args)
	return ret, closer, diags
}
