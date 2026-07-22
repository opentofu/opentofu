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

// BindWorkspaceSelect registers CLI arguments, returning a WorkspaceSelect value and it's corresponding hooks.
func BindWorkspaceSelect(flags Flags) (*WorkspaceSelect, Hooks) {
	var ret WorkspaceSelect
	var hooks Hooks

	ret.ViewOptions.bind(flags, false)
	hooks = append(hooks, ret.ViewOptions.ParseHook())

	ret.Vars = &Vars{}
	ret.Vars.bind(flags)

	flags.BoolVar(&ret.CreateIfMissing, "or-create", false, "Create the OpenTofu workspace if it doesn't exist.")

	return &ret, hooks
}

func ParseWorkspaceSelect(args []string) (*WorkspaceSelect, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	ret, hooks := BindWorkspaceSelect(flags)

	cmdFlags := defaultFlagSet("workspace select", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	// TODO positional args
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

	return ret, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
