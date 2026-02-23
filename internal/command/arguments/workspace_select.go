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

	// ViewOptions contains the options that allows the user to configure different types of outputs
	// from the current command.
	ViewOptions ViewOptions

	// Vars holds the information that might be needed to be given through `-var`/`-var-file`.
	Vars *Vars
}

func ParseWorkspaceSelect(args []string) (*WorkspaceSelect, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret := &WorkspaceSelect{
		Vars: &Vars{},
	}

	cmdFlags := extendedFlagSet("workspace select", nil, nil, ret.Vars)
	cmdFlags.BoolVar(&ret.CreateIfMissing, "or-create", false, "create workspace if it does not exist")
	ret.ViewOptions.AddFlags(cmdFlags, false)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

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

	closer, moreDiags := ret.ViewOptions.Parse()
	diags = diags.Append(moreDiags)
	return ret, closer, diags
}
