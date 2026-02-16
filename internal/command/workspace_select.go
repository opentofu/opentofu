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
	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/posener/complete"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

type WorkspaceSelectCommand struct {
	Meta
	LegacyName bool
}

func (c *WorkspaceSelectCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()

	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)
	// Because the legacy UI was using println to show diagnostics and the new view is using, by default, print,
	// in order to keep functional parity, we setup the view to add a new line after each diagnostic.
	c.View.DiagsWithNewline()

	// Propagate -no-color for legacy use of Ui. The remote backend and
	// cloud package use this; it should be removed when/if they are
	// migrated to views.
	c.Meta.color = !common.NoColor
	c.Meta.Color = c.Meta.color

	// Parse and validate flags
	args, closer, diags := arguments.ParseWorkspaceSelect(rawArgs)
	defer closer()

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewWorkspace(args.ViewOptions, c.View)
	// ... and initialise the Meta.Ui to wrap Meta.View into a new implementation
	// that is able to print by using View abstraction and use the Meta.Ui
	// to ask for the user input.
	c.Meta.configureUiFromView(args.ViewOptions)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		if args.ViewOptions.ViewType == arguments.ViewJSON {
			return 1 // in case it's json, do not print the help of the command
		}
		return cli.RunResultHelp
	}
	c.GatherVariables(args.Vars)

	view.WarnWhenUsedAsEnvCmd(c.LegacyName)

	configPath := c.WorkingDir.NormalizePath(c.WorkingDir.RootModuleDir())

	backendConfig, backendDiags := c.loadBackendConfig(ctx, configPath)
	diags = diags.Append(backendDiags)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	current, isOverridden := c.WorkspaceOverridden(ctx)
	if isOverridden {
		view.WorkspaceIsOverriddenSelectError()
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
		Config: backendConfig,
	}, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// This command will not write state
	c.ignoreRemoteVersionConflict(b)

	name := args.WorkspaceName
	if !validWorkspaceName(name) {
		view.WorkspaceInvalidName(name)
		return 1
	}

	states, err := b.Workspaces(ctx)
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Error loading workspaces",
			fmt.Sprintf("Listing workspaces failed: %s", err),
		)})
		return 1
	}

	if name == current {
		// already using this workspace
		return 0
	}

	found := false
	for _, s := range states {
		if name == s {
			found = true
			break
		}
	}

	var newState bool

	if !found {
		if args.CreateIfMissing {
			_, err = b.StateMgr(ctx, name)
			if err != nil {
				view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
					tfdiags.Error,
					"Error getting the state manager",
					fmt.Sprintf("Failed getting state manager for workspace %s: %s", name, err),
				)})
				return 1
			}
			newState = true
		} else {
			view.WorkspaceDoesNotExist(name)
			return 1
		}
	}

	err = c.SetWorkspace(name)
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Error setting workspace",
			fmt.Sprintf("Could not set the requested workspace: %s", err),
		)})
		return 1
	}

	if newState {
		view.WorkspaceCreated(name)
	} else {
		view.WorkspaceChanged(name)
	}

	return 0
}

func (c *WorkspaceSelectCommand) AutocompleteArgs() complete.Predictor {
	return completePredictSequence{
		c.completePredictWorkspaceName(c.CommandContext()),
		complete.PredictDirs(""),
	}
}

func (c *WorkspaceSelectCommand) AutocompleteFlags() complete.Flags {
	return nil
}

func (c *WorkspaceSelectCommand) Help() string {
	helpText := `
Usage: tofu [global options] workspace select [options] NAME

  Select a different OpenTofu workspace.

Options:

    -or-create=false    Create the OpenTofu workspace if it doesn't exist.

    -var 'foo=bar'       Set a value for one of the input variables in the root
                         module of the configuration. Use this option more than
                         once to set more than one variable.

    -var-file=filename   Load variable values from the given file, in addition
                         to the default files terraform.tfvars and *.auto.tfvars.
                         Use this option more than once to include more than one
                         variables file.
    
    -json                The output of the command is printed in json format.

    -json-into=out.json  Produce the same output as -json, but sent directly
                         to the given file. This allows automation to preserve
                         the original human-readable output streams, while
                         capturing more detailed logs for machine analysis.
`
	return strings.TrimSpace(helpText)
}

func (c *WorkspaceSelectCommand) Synopsis() string {
	return "Select a workspace"
}

// TODO meta-refactor: move this to arguments once all commands are using the same shim logic
func (c *WorkspaceSelectCommand) GatherVariables(args *arguments.Vars) {
	// FIXME the arguments package currently trivially gathers variable related
	// arguments in a heterogeneous slice, in order to minimize the number of
	// code paths gathering variables during the transition to this structure.
	// Once all commands that gather variables have been converted to this
	// structure, we could move the variable gathering code to the arguments
	// package directly, removing this shim layer.

	varArgs := args.All()
	items := make([]flags.RawFlag, len(varArgs))
	for i := range varArgs {
		items[i].Name = varArgs[i].Name
		items[i].Value = varArgs[i].Value
	}
	c.Meta.variableArgs = flags.RawFlags{Items: &items}
}
