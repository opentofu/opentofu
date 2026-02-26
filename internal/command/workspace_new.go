// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/posener/complete"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/clistate"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/statefile"
)

type WorkspaceNewCommand struct {
	Meta
	LegacyName bool
}

func (c *WorkspaceNewCommand) Run(rawArgs []string) int {
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
	args, closer, diags := arguments.ParseWorkspaceNew(rawArgs)
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

	// TODO meta-refactor: remove these when meta state locking related fields are removed and pass the
	//  arguments to the backend component instead
	c.stateLock = args.StateLock
	c.stateLockTimeout = args.StateLockTimeout
	c.statePath = args.StatePath

	configPath := c.WorkingDir.NormalizePath(c.WorkingDir.RootModuleDir())

	workspace := args.WorkspaceName

	if !validWorkspaceName(workspace) {
		view.WorkspaceInvalidName(workspace)
		return 1
	}

	// You can't ask to create a workspace when you're overriding the
	// workspace name to be something different.
	if current, isOverridden := c.WorkspaceOverridden(ctx); current != workspace && isOverridden {
		view.WorkspaceIsOverriddenNewError()
		return 1
	}

	backendConfig, backendDiags := c.loadBackendConfig(ctx, configPath)
	diags = diags.Append(backendDiags)
	if diags.HasErrors() {
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
		Config: backendConfig,
	}, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// This command will not write state
	c.ignoreRemoteVersionConflict(b)

	workspaces, err := b.Workspaces(ctx)
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Error getting existing workspaces",
			fmt.Sprintf("Failed listing workspaces: %s", err),
		)})
		return 1
	}
	for _, ws := range workspaces {
		if workspace == ws {
			view.WorkspaceAlreadyExists(workspace)
			return 1
		}
	}

	_, err = b.StateMgr(ctx, workspace)
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Error getting the state manager",
			fmt.Sprintf("Failed getting state manager for workspace %s: %s", workspace, err),
		)})
		return 1
	}

	// now set the current workspace locally
	if err := c.SetWorkspace(workspace); err != nil {
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Error selecting the new workspace",
			fmt.Sprintf("Failed selecting the new workspace %s: %s", workspace, err),
		)})
		return 1
	}

	view.WorkspaceCreated(workspace)

	statePath := args.StatePath
	if statePath == "" {
		// if we're not loading a state, then we're done
		return 0
	}

	// load the new Backend state
	stateMgr, err := b.StateMgr(ctx, workspace)
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Error getting the state manager for the new workspace",
			fmt.Sprintf("Failed getting state manager for workspace %s: %s", workspace, err),
		)})
		return 1
	}

	if args.StateLock {
		stateLocker := clistate.NewLocker(args.StateLockTimeout, views.NewStateLocker(args.ViewOptions, c.View))
		if diags := stateLocker.Lock(stateMgr, "workspace-new"); diags.HasErrors() {
			view.Diagnostics(diags)
			return 1
		}
		defer func() {
			if diags := stateLocker.Unlock(); diags.HasErrors() {
				view.Diagnostics(diags)
			}
		}()
	}

	// read the existing state file
	f, err := os.Open(statePath)
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Error opening the given state",
			fmt.Sprintf("Failed opening the given state %q: %s", statePath, err),
		)})
		return 1
	}

	stateFile, err := statefile.Read(f, encryption.StateEncryptionDisabled()) // Assume given statefile is not encrypted
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Error reading the given state",
			fmt.Sprintf("Failed reading the given state %q: %s", statePath, err),
		)})
		return 1
	}

	// save the existing state in the new Backend.
	err = stateMgr.WriteState(stateFile.State)
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Error writing the new state to the given state file",
			fmt.Sprintf("Failed writing the new state to the given state file %q: %s", statePath, err),
		)})
		return 1
	}
	err = stateMgr.PersistState(context.TODO(), nil)
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Error persisting the new state",
			fmt.Sprintf("Failed persisting the new state: %s", err),
		)})
		return 1
	}

	return 0
}

func (c *WorkspaceNewCommand) AutocompleteArgs() complete.Predictor {
	return completePredictSequence{
		complete.PredictAnything,
		complete.PredictDirs(""),
	}
}

func (c *WorkspaceNewCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"-state": complete.PredictFiles("*.tfstate"),
	}
}

func (c *WorkspaceNewCommand) Help() string {
	helpText := `
Usage: tofu [global options] workspace new [OPTIONS] NAME

  Create a new OpenTofu workspace.

Options:

    -lock=false         Don't hold a state lock during the operation. This is
                        dangerous if others might concurrently run commands
                        against the same workspace.

    -lock-timeout=0s    Duration to retry a state lock.

    -state=path         Copy an existing state file into the new workspace.


    -var 'foo=bar'      Set a value for one of the input variables in the root
                        module of the configuration. Use this option more than
                        once to set more than one variable.

    -var-file=filename  Load variable values from the given file, in addition
                        to the default files terraform.tfvars and *.auto.tfvars.
                        Use this option more than once to include more than one
                        variables file.
    
    -json               The output of the command is printed in json format.

    -json-into=out.json Produce the same output as -json, but sent directly
                        to the given file. This allows automation to preserve
                        the original human-readable output streams, while
                        capturing more detailed logs for machine analysis.
`
	return strings.TrimSpace(helpText)
}

func (c *WorkspaceNewCommand) Synopsis() string {
	return "Create a new workspace"
}

// TODO meta-refactor: move this to arguments once all commands are using the same shim logic
func (c *WorkspaceNewCommand) GatherVariables(args *arguments.Vars) {
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
