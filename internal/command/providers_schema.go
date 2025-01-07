// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"os"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/jsonprovider"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ProvidersSchemaCommand is a Command implementation that prints out information
// about the providers used in the current configuration/state.
type ProvidersSchemaCommand struct {
	Meta
}

func (c *ProvidersSchemaCommand) Help() string {
	return providersSchemaCommandHelp
}

func (c *ProvidersSchemaCommand) Synopsis() string {
	return "Show schemas for the providers used in the configuration"
}

func (c *ProvidersSchemaCommand) Run(args []string) int {
	ctx := c.CommandContext()

	args = c.Meta.process(args)
	cmdFlags := c.Meta.defaultFlagSet("providers schema")
	c.Meta.varFlagSet(cmdFlags)
	var jsonOutput bool
	cmdFlags.BoolVar(&jsonOutput, "json", false, "produce JSON output")

	cmdFlags.Usage = func() { c.Ui.Error(c.Help()) }
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line flags: %s\n", err.Error()))
		return 1
	}

	if !jsonOutput {
		c.Ui.Error(
			"The `tofu providers schema` command requires the `-json` flag.\n")
		cmdFlags.Usage()
		return 1
	}

	// Check for user-supplied plugin path
	var err error
	if c.pluginPath, err = c.loadPluginPath(); err != nil {
		c.Ui.Error(fmt.Sprintf("Error loading plugin path: %s", err))
		return 1
	}

	var diags tfdiags.Diagnostics

	enc, encDiags := c.Encryption()
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	// Load the backend
	b, backendDiags := c.Backend(nil, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	// We require a local backend
	local, ok := b.(backend.Local)
	if !ok {
		c.showDiagnostics(diags) // in case of any warnings in here
		c.Ui.Error(ErrUnsupportedLocalOp)
		return 1
	}

	// This is a read-only command
	c.ignoreRemoteVersionConflict(b)

	// we expect that the config dir is the cwd
	cwd, err := os.Getwd()
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error getting cwd: %s", err))
		return 1
	}

	// Build the operation
	opReq := c.Operation(b, arguments.ViewJSON, enc)
	opReq.ConfigDir = cwd
	opReq.ConfigLoader, err = c.initConfigLoader()
	var callDiags tfdiags.Diagnostics
	opReq.RootCall, callDiags = c.rootModuleCall(opReq.ConfigDir)
	diags = diags.Append(callDiags)
	if callDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	opReq.AllowUnsetVariables = true
	if err != nil {
		diags = diags.Append(err)
		c.showDiagnostics(diags)
		return 1
	}

	// Get the context
	lr, _, ctxDiags := local.LocalRun(ctx, opReq)
	diags = diags.Append(ctxDiags)
	if ctxDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	schemas, moreDiags := lr.Core.Schemas(lr.Config, lr.InputState)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	jsonSchemas, err := jsonprovider.Marshal(schemas)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Failed to marshal provider schemas to json: %s", err))
		return 1
	}
	c.Ui.Output(string(jsonSchemas))

	return 0
}

const providersSchemaCommandHelp = `
Usage: tofu [global options] providers schema [options] -json

  Prints out a json representation of the schemas for all providers used 
  in the current configuration.

Options:

  -var 'foo=bar'     Set a value for one of the input variables in the root
                     module of the configuration. Use this option more than
                     once to set more than one variable.

  -var-file=filename Load variable values from the given file, in addition
                     to the default files terraform.tfvars and *.auto.tfvars.
                     Use this option more than once to include more than one
                     variables file.
`
