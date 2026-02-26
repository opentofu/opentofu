// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
)

// MetadataFunctionsCommand is a Command implementation that prints out information
// about the available functions in OpenTofu.
type MetadataFunctionsCommand struct {
	Meta
}

func (c *MetadataFunctionsCommand) Help() string {
	return metadataFunctionsCommandHelp
}

func (c *MetadataFunctionsCommand) Synopsis() string {
	return "Show signatures and descriptions for the available functions"
}

func (c *MetadataFunctionsCommand) Run(rawArgs []string) int {
	// new view
	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)

	// Parse and validate flags
	_, closer, diags := arguments.ParseMetadataFunctions(rawArgs)
	defer closer()

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewMetadataFunctions(c.View)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return cli.RunResultHelp
	}

	if !view.PrintFunctions() {
		return 1
	}
	return 0
}

const metadataFunctionsCommandHelp = `
Usage: tofu [global options] metadata functions -json

  Prints out a json representation of the available function signatures.
`
