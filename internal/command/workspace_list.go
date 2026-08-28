// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/posener/complete"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

func WorkspaceListCommander(legacyName bool) Command {
	cmd := Command{
		Name:  "list",
		Short: "List Workspaces",
		Long:  `List OpenTofu workspaces.`,

		DiagsWithNewline: true,
	}

	args := arguments.BindWorkspaceList(&cmd.CommandLine)
	cmd.Run = func(meta Meta) int {
		return WorkspaceListCommand{meta, legacyName}.Execute(args, views.NewWorkspace(args.View, meta.View))
	}

	return cmd
}

type WorkspaceListCommand struct {
	Meta
	LegacyName bool
}

func (c WorkspaceListCommand) Execute(args *arguments.WorkspaceList, view views.Workspace) int {
	var diags tfdiags.Diagnostics

	ctx := c.CommandContext()

	view.WarnWhenUsedAsEnvCmd(c.LegacyName)

	configPath := c.WorkingDir.NormalizePath(c.WorkingDir.RootModuleDir())

	// Load the encryption configuration
	enc, encDiags := c.EncryptionFromPath(ctx, configPath)
	if encDiags.HasErrors() {
		view.Diagnostics(encDiags)
		return 1
	}

	backendConfig, backendDiags := c.loadBackendConfig(ctx, configPath)
	diags = diags.Append(backendDiags)
	if diags.HasErrors() {
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

	workspaces, err := b.Workspaces(ctx)
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Error loading workspaces",
			fmt.Sprintf("Listing workspaces failed: %s", err),
		)})
		return 1
	}

	env, isOverridden := c.WorkspaceOverridden(ctx)
	view.ListWorkspaces(workspaces, env)

	if isOverridden {
		view.WorkspaceOverwrittenByEnvVarWarn()
	}

	return 0
}

func (c *WorkspaceListCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictDirs("")
}

func (c *WorkspaceListCommand) AutocompleteFlags() complete.Flags {
	return nil
}
