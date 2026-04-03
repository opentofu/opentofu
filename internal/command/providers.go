// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ProvidersCommand is a Command implementation that prints out information
// about the providers used in the current configuration/state.
type ProvidersCommand struct {
	Meta
}

func (c *ProvidersCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()

	// new view
	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)
	// Because the legacy UI was using println to show diagnostics and the new view is using, by default, print,
	// in order to keep functional parity, we setup the view to add a new line after each diagnostic.
	c.View.DiagsWithNewline()

	// Parse and validate flags
	args, closer, diags := arguments.ParseProviders(rawArgs)
	defer closer()

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewProviders(c.View)
	// ... and initialise the Meta.Ui to wrap Meta.View into a new implementation
	// that is able to print by using View abstraction and use the Meta.Ui
	// to ask for the user input.
	c.Meta.configureUiFromView(args.ViewOptions)

	if diags.HasErrors() {
		view.Diagnostics(diags)
		return cli.RunResultHelp
	}
	c.Meta.variableArgs = args.Vars.All()
	// This gets the current directory as full path.
	configPath := c.WorkingDir.NormalizePath(c.WorkingDir.RootModuleDir())

	empty, err := configs.IsEmptyDir(configPath)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error validating configuration directory",
			fmt.Sprintf("OpenTofu encountered an unexpected error while verifying that the given configuration directory is valid: %s.", err),
		))
		view.Diagnostics(diags)
		return 1
	}
	if empty {
		absPath, err := filepath.Abs(configPath)
		if err != nil {
			absPath = configPath
		}
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"No configuration files",
			fmt.Sprintf("The directory %s contains no OpenTofu configuration files.", absPath),
		))
		view.Diagnostics(diags)
		return 1
	}

	config, configDiags := c.loadConfigWithTests(ctx, configPath, args.TestsDirectory)
	diags = diags.Append(configDiags)
	if configDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Load the encryption configuration
	enc, encDiags := c.EncryptionFromPath(ctx, configPath)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Load the backend
	b, backendDiags := c.Backend(ctx, &BackendOpts{
		Config: config.Module.Backend,
		View:   view.Backend(),
	}, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// This is a read-only command
	c.ignoreRemoteVersionConflict(b)

	// Get the state
	env, err := c.Workspace(ctx)
	if err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed getting the current workspace",
			fmt.Sprintf("Error selecting workspace: %s", err),
		)))
		return 1
	}
	s, err := b.StateMgr(ctx, env)
	if err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed loading state",
			fmt.Sprintf("Failed to load state: %s", err),
		)))
		return 1
	}
	if err := s.RefreshState(context.TODO()); err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed refreshing the state",
			fmt.Sprintf("Failed to refresh state: %s", err),
		)))
		return 1
	}

	reqs, reqDiags := config.ProviderRequirementsByModule()
	diags = diags.Append(reqDiags)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}
	view.ModuleRequirements(reqs)

	state := s.State()
	var stateReqs getproviders.Requirements
	if state != nil {
		stateReqs = state.ProviderRequirements()
	}
	view.StateRequirements(stateReqs)

	view.Diagnostics(diags)
	if diags.HasErrors() {
		return 1
	}
	return 0
}

func (c *ProvidersCommand) Help() string {
	return providersCommandHelp
}

func (c *ProvidersCommand) Synopsis() string {
	return "Show the providers required for this configuration"
}

const providersCommandHelp = `
Usage: tofu [global options] providers [options] [DIR]

  Prints out a tree of modules in the referenced configuration annotated with
  their provider requirements.

  This provides an overview of all of the provider requirements across all
  referenced modules, as an aid to understanding why particular provider
  plugins are needed and why particular versions are selected.

Options:

  -test-directory=path  Set the OpenTofu test directory, defaults to "tests". When set, the
                        test command will search for test files in the current directory and
                        in the one specified by the flag.

  -var 'foo=bar'        Set a value for one of the input variables in the root
                        module of the configuration. Use this option more than
                        once to set more than one variable.

  -var-file=filename    Load variable values from the given file, in addition
                        to the default files terraform.tfvars and *.auto.tfvars.
                        Use this option more than once to include more than one
                        variables file.
`
