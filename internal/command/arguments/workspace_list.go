// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type WorkspaceList struct {
	// View represents the global view options
	View *View

	// Vars holds the information that might be needed to be given through `-var`/`-var-file`.
	Vars *Vars
}

// BindWorkspaceList registers CLI arguments, returning a WorkspaceList value and it's corresponding hooks.
func BindWorkspaceList(cli *CommandLine) *WorkspaceList {
	return &WorkspaceList{
		View: BindView(cli, viewFlagNoInput),
		Vars: BindVars(cli),
	}
}

func ParseWorkspaceList(args []string) (*WorkspaceList, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindWorkspaceList(cli)
	closer, diags := cli.parseWithHooks("workspace list", args)
	return ret, closer, diags
}
