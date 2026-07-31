// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type WorkspaceShow struct {
	// ViewOptions contains the options that allows the user to configure different types of outputs
	// from the current command.
	ViewOptions ViewOptions

	// Vars holds the information that might be needed to be given through `-var`/`-var-file`.
	Vars *Vars
}

// BindWorkspaceShow registers CLI arguments, returning a WorkspaceShow value and it's corresponding hooks.
func BindWorkspaceShow(cli *CommandLine) *WorkspaceShow {
	var ret WorkspaceShow

	// we only parse but do not register the views flags since this command does not need it
	ret.ViewOptions.ParseHook(cli)

	ret.Vars = BindVars(cli)

	return &ret
}

func ParseWorkspaceShow(args []string) (*WorkspaceShow, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindWorkspaceShow(cli)
	closer, diags := cli.Stdlib("workspace show", args)
	return ret, closer, diags
}
