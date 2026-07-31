// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type WorkspaceList struct {
	// ViewOptions contains the options that allows the user to configure different types of outputs
	// from the current command.
	ViewOptions ViewOptions

	// Vars holds the information that might be needed to be given through `-var`/`-var-file`.
	Vars *Vars
}

// BindWorkspaceList registers CLI arguments, returning a WorkspaceList value and it's corresponding hooks.
func BindWorkspaceList(cli *CommandLine) *WorkspaceList {
	var ret WorkspaceList

	ret.ViewOptions.bind(cli, false)

	ret.Vars = BindVars(cli)

	return &ret
}

func ParseWorkspaceList(args []string) (*WorkspaceList, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindWorkspaceList(cli)
	closer, diags := cli.Stdlib("workspace list", args)
	return ret, closer, diags
}
