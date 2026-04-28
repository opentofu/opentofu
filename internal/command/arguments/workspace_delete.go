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

func ParseWorkspaceDelete(args []string) (*WorkspaceDelete, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret := &WorkspaceDelete{
		Vars:  &Vars{},
		State: &State{},
	}

	cmdFlags := extendedFlagSet("workspace delete", nil, ret.Vars)
	cmdFlags.BoolVar(&ret.Force, "force", false, "force removal of a non-empty workspace")
	ret.State.AddFlags(cmdFlags, true, false, false, false)
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
