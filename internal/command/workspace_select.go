// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"slices"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/posener/complete"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

func WorkspaceSelectCommander(legacyName bool) Command {
	cmd := Command{
		Name:  "select",
		Short: "Select a workspace",
		Long:  `Select a different OpenTofu workspace.`,

		DiagsWithNewline: true,
	}

	args := arguments.BindWorkspaceSelect(&cmd.CommandLine)
	cmd.Run = func(meta Meta) int {
		return WorkspaceSelectCommand{meta, legacyName}.Execute(args, views.NewWorkspace(args.View, meta.View))
	}

	return cmd
}

type WorkspaceSelectCommand struct {
	Meta
	LegacyName bool
}

func (c WorkspaceSelectCommand) Execute(args *arguments.WorkspaceSelect, view views.Workspace) int {
	var diags tfdiags.Diagnostics
	ctx := c.CommandContext()

	view.WarnWhenUsedAsEnvCmd(c.LegacyName)

	configPath := c.WorkingDir.NormalizePath(c.WorkingDir.RootModuleDir())

	backendConfig, backendDiags := c.loadBackendConfig(ctx, configPath)
	diags = diags.Append(backendDiags)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	current, isOverridden := c.WorkspaceOverridden(ctx)
	if isOverridden {
		view.WorkspaceIsOverriddenSelectError()
		return 1
	}

	// Load the encryption configuration
	enc, encDiags := c.EncryptionFromPath(ctx, configPath)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Load the backend
	b, backendDiags := c.Backend(ctx, &BackendOpts{
		Config: backendConfig,
		View:   view.Backend(),
	}, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// This command will not write state
	c.ignoreRemoteVersionConflict(b)

	name := args.WorkspaceName
	if !validWorkspaceName(name) {
		view.WorkspaceInvalidName(name)
		return 1
	}

	states, err := b.Workspaces(ctx)
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Error loading workspaces",
			fmt.Sprintf("Listing workspaces failed: %s", err),
		)})
		return 1
	}

	if name == current {
		// already using this workspace
		return 0
	}

	found := slices.Contains(states, name)

	var newState bool

	if !found {
		if args.CreateIfMissing {
			_, err = b.StateMgr(ctx, name)
			if err != nil {
				view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
					tfdiags.Error,
					"Error getting the state manager",
					fmt.Sprintf("Failed getting state manager for workspace %s: %s", name, err),
				)})
				return 1
			}
			newState = true
		} else {
			view.WorkspaceDoesNotExist(name)
			return 1
		}
	}

	err = c.SetWorkspace(name)
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Error setting workspace",
			fmt.Sprintf("Could not set the requested workspace: %s", err),
		)})
		return 1
	}

	if newState {
		view.WorkspaceCreated(name)
	} else {
		view.WorkspaceChanged(name)
	}

	return 0
}

func (c *WorkspaceSelectCommand) AutocompleteArgs() complete.Predictor {
	return completePredictSequence{
		c.completePredictWorkspaceName(c.CommandContext()),
		complete.PredictDirs(""),
	}
}

func (c *WorkspaceSelectCommand) AutocompleteFlags() complete.Flags {
	return nil
}
