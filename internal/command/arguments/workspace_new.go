// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"time"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

type WorkspaceNew struct {
	// Workspace represents the name of the workspace that the user wants to be selected.
	WorkspaceName string

	// StatePath allows the user to give a specific state file into the command.
	StatePath string
	// StateLock allows the user to disable, the default enabled, state locking.
	StateLock bool
	// StateLockTimeout allows the user to configure the timeout for the locking of the state..
	StateLockTimeout time.Duration

	// ViewOptions contains the options that allows the user to configure different types of outputs
	// from the current command.
	ViewOptions ViewOptions

	// Vars holds the information that might be needed to be given through `-var`/`-var-file`.
	Vars *Vars
}

func ParseWorkspaceNew(args []string) (*WorkspaceNew, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret := &WorkspaceNew{
		Vars: &Vars{},
	}

	cmdFlags := extendedFlagSet("workspace new", nil, nil, ret.Vars)
	cmdFlags.StringVar(&ret.StatePath, "state", "", "tofu state file")
	cmdFlags.BoolVar(&ret.StateLock, "lock", true, "lock state")
	cmdFlags.DurationVar(&ret.StateLockTimeout, "lock-timeout", 0, "lock timeout")
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
