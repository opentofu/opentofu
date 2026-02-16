// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/posener/complete"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/clistate"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type WorkspaceDeleteCommand struct {
	Meta
	LegacyName bool
}

func (c *WorkspaceDeleteCommand) Run(rawArgs []string) int {
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
	args, closer, diags := arguments.ParseWorkspaceDelete(rawArgs)
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
			"Error loading workspaces",
			fmt.Sprintf("Listing workspaces failed: %s", err),
		)})
		return 1
	}

	workspace := args.WorkspaceName
	exists := false
	for _, ws := range workspaces {
		if workspace == ws {
			exists = true
			break
		}
	}

	if !exists {
		view.WorkspaceDoesNotExist(workspace)
		return 1
	}

	currentWorkspace, err := c.Workspace(ctx)
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Error getting the current workspace",
			fmt.Sprintf("Failed getting the current workspace: %s", err),
		)})
		return 1
	}
	if workspace == currentWorkspace {
		view.CannotDeleteCurrentWorkspace(workspace)
		return 1
	}

	// we need the actual state to see if it's empty
	stateMgr, err := b.StateMgr(ctx, workspace)
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to fetch the state",
			fmt.Sprintf("Fetching state failed: %s", err),
		)})
		return 1
	}

	var stateLocker clistate.Locker
	if args.StateLock {
		stateLocker = clistate.NewLocker(c.stateLockTimeout, views.NewStateLocker(args.ViewOptions, c.View))
		if diags := stateLocker.Lock(stateMgr, "state-replace-provider"); diags.HasErrors() {
			view.Diagnostics(diags)
			return 1
		}
	} else {
		stateLocker = clistate.NewNoopLocker()
	}

	if err := stateMgr.RefreshState(context.TODO()); err != nil {
		// We need to release the lock before exit
		stateLocker.Unlock()
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to refresh state",
			fmt.Sprintf("State refresh failed: %s", err),
		)})
		return 1
	}

	hasResources := stateMgr.State().HasManagedResourceInstanceObjects()

	if hasResources && !args.Force {
		// We'll collect a list of what's being managed here as extra context
		// for the message.
		var buf strings.Builder
		for _, obj := range stateMgr.State().AllResourceInstanceObjectAddrs() {
			if obj.DeposedKey == states.NotDeposed {
				fmt.Fprintf(&buf, "\n  - %s", obj.Instance.String())
			} else {
				fmt.Fprintf(&buf, "\n  - %s (deposed object %s)", obj.Instance.String(), obj.DeposedKey)
			}
		}

		// We need to release the lock before exit
		stateLocker.Unlock()

		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Workspace is not empty",
			fmt.Sprintf(
				"Workspace %q is currently tracking the following resource instances:%s\n\nDeleting this workspace would cause OpenTofu to lose track of any associated remote objects, which would then require you to delete them manually outside of OpenTofu. You should destroy these objects with OpenTofu before deleting the workspace.\n\nIf you want to delete this workspace anyway, and have OpenTofu forget about these managed objects, use the -force option to disable this safety check.",
				workspace, buf.String(),
			),
		))
		view.Diagnostics(diags)
		return 1
	}

	// We need to release the lock just before deleting the state, in case
	// the backend can't remove the resource while holding the lock. This
	// is currently true for Windows local files.
	//
	// TODO: While there is little safety in locking while deleting the
	// state, it might be nice to be able to coordinate processes around
	// state deletion, i.e. in a CI environment. Adding Delete() as a
	// required method of States would allow the removal of the resource to
	// be delegated from the Backend to the State itself.
	stateLocker.Unlock()

	err = b.DeleteWorkspace(ctx, workspace, args.Force)
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
			tfdiags.Error,
			"Workspace deletion failed",
			fmt.Sprintf("Failed to delete the given workspace: %s", err),
		)})
		return 1
	}

	view.WorkspaceDeleted(workspace)

	if hasResources {
		view.DeletedWorkspaceNotEmpty(workspace)
	}

	return 0
}

func (c *WorkspaceDeleteCommand) AutocompleteArgs() complete.Predictor {
	return completePredictSequence{
		c.completePredictWorkspaceName(c.CommandContext()),
		complete.PredictDirs(""),
	}
}

func (c *WorkspaceDeleteCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"-force": complete.PredictNothing,
	}
}

func (c *WorkspaceDeleteCommand) Help() string {
	helpText := `
Usage: tofu [global options] workspace delete [options] NAME

  Delete a OpenTofu workspace


Options:

  -force               Remove a workspace even if it is managing resources.
                       OpenTofu can no longer track or manage the workspace's
                       infrastructure.

  -lock=false          Don't hold a state lock during the operation. This is
                       dangerous if others might concurrently run commands
                       against the same workspace.

  -lock-timeout=0s     Duration to retry a state lock.

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

func (c *WorkspaceDeleteCommand) Synopsis() string {
	return "Delete a workspace"
}

// TODO meta-refactor: move this to arguments once all commands are using the same shim logic
func (c *WorkspaceDeleteCommand) GatherVariables(args *arguments.Vars) {
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
