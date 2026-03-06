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
	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/command/jsonprovider"
	"github.com/opentofu/opentofu/internal/command/views"
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

func (c *ProvidersSchemaCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()

	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)

	// Propagate -no-color for legacy use of Ui. The remote backend and
	// cloud package use this; it should be removed when/if they are
	// migrated to views.
	c.Meta.color = !common.NoColor
	c.Meta.Color = c.Meta.color

	args, closer, diags := arguments.ParseProvidersSchema(rawArgs)
	defer closer()

	view := views.NewProvidersSchema(args.ViewOptions, c.View)

	c.Meta.configureUiFromView(args.ViewOptions)

	if diags.HasErrors() {
		view.HelpPrompt()
		view.Diagnostics(diags)
		return 1
	}

	c.GatherVariables(args.Vars)

	// Check for user-supplied plugin path
	var err error
	if c.pluginPath, err = c.loadPluginPath(); err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Plugins loading error",
			fmt.Sprintf("Error loading plugin path: %s", err),
		)))
		return 1
	}

	enc, encDiags := c.Encryption(ctx)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Load the backend
	b, backendDiags := c.Backend(ctx, nil, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// We require a local backend
	local, ok := b.(backend.Local)
	if !ok {
		view.Diagnostics(diags) // in case of any warnings in here
		view.UnsupportedLocalOp()
		return 1
	}

	// This is a read-only command
	c.ignoreRemoteVersionConflict(b)

	// we expect that the config dir is the cwd
	cwd, err := os.Getwd()
	if err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error getting cwd",
			err.Error(),
		)))
		return 1
	}

	// Build the operation
	opReq := c.Operation(ctx, b, args.ViewOptions, enc)
	opReq.ConfigDir = cwd
	opReq.ConfigLoader, err = c.initConfigLoader()
	var callDiags tfdiags.Diagnostics
	opReq.RootCall, callDiags = c.rootModuleCall(ctx, opReq.ConfigDir)
	diags = diags.Append(callDiags)
	if callDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	opReq.AllowUnsetVariables = true
	if err != nil {
		diags = diags.Append(err)
		view.Diagnostics(diags)
		return 1
	}

	// Get the context
	lr, _, ctxDiags := local.LocalRun(ctx, opReq)
	diags = diags.Append(ctxDiags)
	if ctxDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	schemas, moreDiags := lr.Core.Schemas(ctx, lr.Config, lr.InputState)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	jsonSchemas, err := jsonprovider.Marshal(schemas)
	if err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to marshal provider schemas to json",
			err.Error(),
		)))
		return 1
	}

	view.Output(string(jsonSchemas))

	return 0
}

// TODO meta-refactor: move this to arguments once all commands are using the same shim logic
func (c *ProvidersSchemaCommand) GatherVariables(args *arguments.Vars) {
	varArgs := args.All()
	items := make([]flags.RawFlag, len(varArgs))
	for i := range varArgs {
		items[i].Name = varArgs[i].Name
		items[i].Value = varArgs[i].Value
	}
	c.Meta.variableArgs = flags.RawFlags{Items: &items}
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
