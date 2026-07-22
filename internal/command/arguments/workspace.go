// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Workspace struct {
	// ViewOptions contains the options that allows the user to configure different types of outputs
	// from the current command.
	ViewOptions ViewOptions
}

// BindWorkspace registers CLI arguments, returning a Workspace value and it's corresponding hooks.
func BindWorkspace(flags Flags) (*Workspace, Hooks) {
	var get Workspace
	var hooks Hooks

	// we only parse but do not register the views flags since this command does not need it
	hooks = append(hooks, get.ViewOptions.ParseHook())

	return &get, hooks
}

func ParseWorkspace(args []string) (*Workspace, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	ret, hooks := BindWorkspace(flags)

	cmdFlags := defaultFlagSet("workspace", flags)

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

	return ret, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
