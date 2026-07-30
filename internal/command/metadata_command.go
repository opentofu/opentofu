// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"strings"

	"github.com/mitchellh/cli"
)

func MetadataCommander() Command {
	cmd := Command{
		Name:  "metadata",
		Short: "Metadata related commands",
		Long:  "This command has subcommands for metadata related purposes.",

		Commands: []Command{MetadataFunctionsCommander()},
	}

	return cmd
}

// MetadataCommand is a Command implementation that just shows help for
// the subcommands nested below it.
type MetadataCommand struct {
	Meta
}

func (c *MetadataCommand) Run(args []string) int {
	return cli.RunResultHelp
}

func (c *MetadataCommand) Help() string {
	helpText := `
Usage: tofu [global options] metadata <subcommand> [options] [args]

  This command has subcommands for metadata related purposes.

`
	return strings.TrimSpace(helpText)
}

func (c *MetadataCommand) Synopsis() string {
	return "Metadata related commands"
}
