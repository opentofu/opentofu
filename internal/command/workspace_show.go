// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/posener/complete"
)

func WorkspaceShowCommander() Command {
	cmd := Command{
		Name:  "show",
		Short: "Show the name of the current workspace.",
		Long:  `Show the name of the current workspace.`,

		DiagsWithNewline: true,
	}

	args := arguments.BindWorkspaceShow(&cmd.CommandLine)
	cmd.Run = func(meta Meta) int {
		return WorkspaceShowCommand{meta}.Execute(args, views.NewWorkspace(args.View, meta.View))
	}

	return cmd
}

type WorkspaceShowCommand struct {
	Meta
}

func (c *WorkspaceShowCommand) Run(rawArgs []string) int {
	return RunCommand(WorkspaceShowCommander(), c.Meta, rawArgs)
}
func (c WorkspaceShowCommand) Execute(args *arguments.WorkspaceShow, view views.Workspace) int {
	ctx := c.CommandContext()

	workspace, err := c.Workspace(ctx)
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Error getting the current workspace",
			fmt.Sprintf("Failed getting the current workspace: %s", err),
		)})
		return 1
	}
	view.WorkspaceShow(workspace)

	return 0
}

func (c *WorkspaceShowCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictNothing
}

func (c *WorkspaceShowCommand) AutocompleteFlags() complete.Flags {
	return nil
}

func (c *WorkspaceShowCommand) Help() string {
	helpText := `
Usage: tofu [global options] workspace show

  Show the name of the current workspace.
`
	return strings.TrimSpace(helpText)
}

func (c *WorkspaceShowCommand) Synopsis() string {
	return "Show the name of the current workspace"
}
