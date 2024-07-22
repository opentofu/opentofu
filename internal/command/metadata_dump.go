// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/command/jsonconfig"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

var (
	ignoredDump = []string{"map", "list"}
)

// MetadataDumpCommand is a Command implementation that prints out information
// about the config
type MetadataDumpCommand struct {
	Meta
}

func (c *MetadataDumpCommand) Help() string {
	return metadataDumpCommandHelp
}

func (c *MetadataDumpCommand) Synopsis() string {
	return "Show signatures and descriptions for the available config"
}

func (c *MetadataDumpCommand) Run(args []string) int {
	args = c.Meta.process(args)
	cmdFlags := c.Meta.defaultFlagSet("metadata dump")
	var jsonOutput bool
	cmdFlags.BoolVar(&jsonOutput, "json", false, "produce JSON output")

	cmdFlags.Usage = func() { c.Ui.Error(c.Help()) }
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line flags: %s\n", err.Error()))
		return 1
	}

	if !jsonOutput {
		c.Ui.Error(
			"The `tofu metadata dump` command requires the `-json` flag.\n")
		cmdFlags.Usage()
		return 1
	}

	var diags tfdiags.Diagnostics
	loader, err := c.initConfigLoader()
	if err != nil {
		diags = diags.Append(err)
		c.showDiagnostics(diags)
		return 1
	}

	call, vDiags := c.rootModuleCall(".")
	diags = diags.Append(vDiags)
	if diags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	module, hclDiags := loader.Parser().LoadConfigDir(".", call)
	diags = diags.Append(hclDiags)
	if diags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	data, err := jsonconfig.Marshal(&configs.Config{Module: module}, nil)
	if err != nil {
		diags = diags.Append(err)
		c.showDiagnostics(diags)
		return 1
	}

	println(string(data))

	return 0
}

const metadataDumpCommandHelp = `
Usage: tofu [global options] metadata dump -json

  Prints out a json representation of the available config data.
`
