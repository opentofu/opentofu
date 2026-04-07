// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/posener/complete"
)

type WorkspaceShowCommand struct {
	Meta
}

func (c *WorkspaceShowCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()

	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)
	// Because the legacy UI was using println to show diagnostics and the new view is using, by default, print,
	// in order to keep functional parity, we setup the view to add a new line after each diagnostic.
	c.View.DiagsWithNewline()

	// Parse and validate flags
	args, closer, diags := arguments.ParseWorkspaceShow(rawArgs)
	defer closer()

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewWorkspace(args.ViewOptions, c.View)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		if args.ViewOptions.ViewType == arguments.ViewJSON {
			return 1 // in case it's json, do not print the help of the command
		}
		return cli.RunResultHelp
	}
	c.Meta.variableArgs = args.Vars.All()

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
