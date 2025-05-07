// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/clistate"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type WorkspaceNewCommand struct {
	Meta
	LegacyName bool
}

func (c *WorkspaceNewCommand) Run(args []string) int {
	ctx := c.CommandContext()
	args = c.Meta.process(args)
	envCommandShowWarning(c.Ui, c.LegacyName)

	var stateLock bool
	var stateLockTimeout time.Duration
	var statePath string
	cmdFlags := c.Meta.defaultFlagSet("workspace new")
	c.Meta.varFlagSet(cmdFlags)
	cmdFlags.BoolVar(&stateLock, "lock", true, "lock state")
	cmdFlags.DurationVar(&stateLockTimeout, "lock-timeout", 0, "lock timeout")
	cmdFlags.StringVar(&statePath, "state", "", "tofu state file")
	cmdFlags.Usage = func() { c.Ui.Error(c.Help()) }
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line flags: %s\n", err.Error()))
		return 1
	}

	args = cmdFlags.Args()
	if len(args) != 1 {
		c.Ui.Error("Expected a single argument: NAME.\n")
		return cli.RunResultHelp
	}

	workspace := args[0]

	if !validWorkspaceName(workspace) {
		c.Ui.Error(fmt.Sprintf(envInvalidName, workspace))
		return 1
	}

	// You can't ask to create a workspace when you're overriding the
	// workspace name to be something different.
	if current, isOverridden := c.WorkspaceOverridden(ctx); current != workspace && isOverridden {
		c.Ui.Error(envIsOverriddenNewError)
		return 1
	}

	configPath, err := modulePath(args[1:])
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	var diags tfdiags.Diagnostics

	backendConfig, backendDiags := c.loadBackendConfig(ctx, configPath)
	diags = diags.Append(backendDiags)
	if diags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	// Load the encryption configuration
	enc, encDiags := c.EncryptionFromPath(ctx, configPath)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	// Load the backend
	b, backendDiags := c.Backend(ctx, &BackendOpts{
		Config: backendConfig,
	}, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	// This command will not write state
	c.ignoreRemoteVersionConflict(b)

	workspaces, err := b.Workspaces(ctx)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Failed to get configured named states: %s", err))
		return 1
	}
	for _, ws := range workspaces {
		if workspace == ws {
			c.Ui.Error(fmt.Sprintf(envExists, workspace))
			return 1
		}
	}

	_, err = b.StateMgr(ctx, workspace)
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	// now set the current workspace locally
	if err := c.SetWorkspace(workspace); err != nil {
		c.Ui.Error(fmt.Sprintf("Error selecting new workspace: %s", err))
		return 1
	}

	c.Ui.Output(c.Colorize().Color(fmt.Sprintf(
		strings.TrimSpace(envCreated), workspace)))

	if statePath == "" {
		// if we're not loading a state, then we're done
		return 0
	}

	// load the new Backend state
	stateMgr, err := b.StateMgr(ctx, workspace)
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	if stateLock {
		stateLocker := clistate.NewLocker(c.stateLockTimeout, views.NewStateLocker(arguments.ViewHuman, c.View))
		if diags := stateLocker.Lock(stateMgr, "workspace-new"); diags.HasErrors() {
			c.showDiagnostics(diags)
			return 1
		}
		defer func() {
			if diags := stateLocker.Unlock(); diags.HasErrors() {
				c.showDiagnostics(diags)
			}
		}()
	}

	// read the existing state file
	f, err := os.Open(statePath)
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	stateFile, err := statefile.Read(f, encryption.StateEncryptionDisabled()) // Assume given statefile is not encrypted
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	// save the existing state in the new Backend.
	err = stateMgr.WriteState(stateFile.State)
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}
	err = stateMgr.PersistState(nil)
	if err != nil {
		c.Ui.Error(err.Error())
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
`
	return strings.TrimSpace(helpText)
}

func (c *WorkspaceNewCommand) Synopsis() string {
	return "Create a new workspace"
}
