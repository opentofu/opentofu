// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/command/jsonfunction"
	"github.com/opentofu/opentofu/internal/lang"
)

var (
	ignoredFunctions = []addrs.Function{
		addrs.ParseFunction("map").FullyQualified(),
		addrs.ParseFunction("list").FullyQualified(),
	}
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

func (c *MetadataFunctionsCommand) Run(args []string) int {
	args = c.Meta.process(args)
	cmdFlags := c.Meta.defaultFlagSet("metadata functions")
	var jsonOutput bool
	cmdFlags.BoolVar(&jsonOutput, "json", false, "produce JSON output")

	cmdFlags.Usage = func() { c.Ui.Error(c.Help()) }
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line flags: %s\n", err.Error()))
		return 1
	}

	if !jsonOutput {
		c.Ui.Error(
			"The `tofu metadata functions` command requires the `-json` flag.\n")
		cmdFlags.Usage()
		return 1
	}

	scope := &lang.Scope{}
	funcs := scope.Functions()
	filteredFuncs := make(map[string]function.Function)
	for k, v := range funcs {
		if isIgnoredFunction(k) {
			continue
		}
		filteredFuncs[k] = v
	}

	jsonFunctions, marshalDiags := jsonfunction.Marshal(filteredFuncs)
	if marshalDiags.HasErrors() {
		c.showDiagnostics(marshalDiags)
		return 1
	}
	c.Ui.Output(string(jsonFunctions))

	return 0
}

const metadataFunctionsCommandHelp = `
Usage: tofu [global options] metadata functions -json

  Prints out a json representation of the available function signatures.
`

func isIgnoredFunction(name string) bool {
	funcAddr := addrs.ParseFunction(name).FullyQualified().String()
	for _, i := range ignoredFunctions {
		if funcAddr == i.String() {
			return true
		}
	}
	return false
}
