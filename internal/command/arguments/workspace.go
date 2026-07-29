// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Workspace struct {
	// View represents the global view options
	View *View
}

// BindWorkspace registers CLI arguments, returning a Workspace value and it's corresponding hooks.
func BindWorkspace(cli *CommandLine) *Workspace {
	var get Workspace

	get.View = BindView(cli, viewFlagNone)

	return &get
}

func ParseWorkspace(args []string) (*Workspace, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindWorkspace(cli)
	closer, diags := cli.Stdlib("workspace", args)
	return ret, closer, diags
}
