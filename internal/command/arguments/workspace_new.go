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

func ParseWorkspaceNew(args []string) (*WorkspaceNew, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret := &WorkspaceNew{
		Vars:  &Vars{},
		State: &State{},
	}

	cmdFlags := extendedFlagSet("workspace new", nil, ret.Vars)
	ret.State.addFlags(cmdFlags, stateFlagLock|stateFlagStateIn)
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
