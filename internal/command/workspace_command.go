// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"net/url"
	"strings"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
)

func WorkspaceCommander(legacyName bool) Command {
	cmd := Command{
		Name:  "workspace",
		Short: "Workspace management",
		Long:  `new, list, show, select and delete OpenTofu workspaces.`,

		Commands: []Command{
			WorkspaceListCommander(legacyName),
			WorkspaceSelectCommander(legacyName),
			WorkspaceNewCommander(legacyName),
			WorkspaceDeleteCommander(legacyName),
		},

		DiagsWithNewline: true,
	}
	if legacyName {
		cmd.Name = "env"
		cmd.Hidden = true
	} else {
		cmd.Commands = append(cmd.Commands, WorkspaceShowCommander())
	}

	args := arguments.BindWorkspace(&cmd.CommandLine)
	cmd.Run = func(meta Meta) int {
		view := views.NewWorkspace(args.View, meta.View)
		view.WarnWhenUsedAsEnvCmd(legacyName)
		return RunResultHelp
	}

	return cmd
}

// WorkspaceCommand is a Command Implementation that manipulates workspaces,
// which allow multiple distinct states and variables from a single config.
type WorkspaceCommand struct {
	Meta
	LegacyName bool
}

func (c *WorkspaceCommand) Run(rawArgs []string) int {
	return RunCommand(WorkspaceCommander(c.LegacyName), c.Meta, rawArgs)
}

func (c *WorkspaceCommand) Help() string {
	helpText := `
Usage: tofu [global options] workspace

  new, list, show, select and delete OpenTofu workspaces.

`
	return strings.TrimSpace(helpText)
}

func (c *WorkspaceCommand) Synopsis() string {
	return "Workspace management"
}

// validWorkspaceName returns true is this name is valid to use as a workspace name.
// Since most named states are accessed via a filesystem path or URL, check if
// escaping the name would be required.
func validWorkspaceName(name string) bool {
	return name == url.PathEscape(name)
}
