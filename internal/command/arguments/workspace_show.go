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

func ParseWorkspaceShow(args []string) (*WorkspaceShow, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret := &WorkspaceShow{
		Vars: &Vars{},
	}

	cmdFlags := extendedFlagSet("workspace show", nil, nil, ret.Vars)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	if len(cmdFlags.Args()) > 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Unexpected argument",
			"Too many command line arguments. Did you mean to use -chdir?",
		))
	}

	// we only parse but do not register the views flags since this command does not need it
	closer, moreDiags := ret.ViewOptions.Parse()
	diags = diags.Append(moreDiags)
	return ret, closer, diags
}
