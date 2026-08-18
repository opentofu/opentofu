// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type WorkspaceShow struct {
	// View represents the global view options
	View *View

	// Vars holds the information that might be needed to be given through `-var`/`-var-file`.
	Vars *Vars
}

// BindWorkspaceShow registers CLI arguments, returning a WorkspaceShow value and it's corresponding hooks.
func BindWorkspaceShow(cli *CommandLine) *WorkspaceShow {
	return &WorkspaceShow{
		View: BindView(cli, viewFlagNone),
		Vars: BindVars(cli),
	}
}

func ParseWorkspaceShow(args []string) (*WorkspaceShow, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindWorkspaceShow(cli)
	closer, diags := cli.parseWithHooks("workspace show", args)
	return ret, closer, diags
}
