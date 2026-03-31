// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// StatePullCommand is a Command implementation that shows a single resource.
type StatePullCommand struct {
	Meta
	StateMeta
}

func (c *StatePullCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()

	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)
	// Because the legacy UI was using println to show diagnostics and the new view is using, by default, print,
	// in order to keep functional parity, we setup the view to add a new line after each diagnostic.
	c.View.DiagsWithNewline()

	// Parse and validate flags
	args, closer, diags := arguments.ParseStatePull(rawArgs)
	defer closer()

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewState(args.ViewOptions, c.View)
	// ... and initialise the Meta.Ui to wrap Meta.View into a new implementation
	// that is able to print by using View abstraction and use the Meta.Ui
	// to ask for the user input.
	c.Meta.configureUiFromView(args.ViewOptions)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return cli.RunResultHelp
	}

	c.GatherVariables(args.Vars)

	if diags := c.Meta.checkRequiredVersion(ctx); diags != nil {
		view.Diagnostics(diags)
		return 1
	}

	// Load the encryption configuration
	enc, encDiags := c.Encryption(ctx)
	if encDiags.HasErrors() {
		view.Diagnostics(encDiags)
		return 1
	}

	// Load the backend
	b, backendDiags := c.Backend(ctx, nil, enc.State())
	if backendDiags.HasErrors() {
		view.Diagnostics(backendDiags)
		return 1
	}

	// This is a read-only command
	c.ignoreRemoteVersionConflict(b)

	// Get the state manager for the current workspace
	env, err := c.Workspace(ctx)
	if err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error selecting workspace",
			err.Error(),
		)))
		return 1
	}
	stateMgr, err := b.StateMgr(ctx, env)
	if err != nil {
		view.StateLoadingFailure(err.Error())
		return 1
	}
	if err := stateMgr.RefreshState(context.TODO()); err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Failed to refresh state: %s", err)))
		return 1
	}

	// Get a statefile object representing the latest snapshot
	stateFile := statemgr.Export(stateMgr)

	if stateFile != nil { // we produce no output if the statefile is nil
		var buf bytes.Buffer
		err = statefile.Write(stateFile, &buf, encryption.StateEncryptionDisabled()) // Don't encrypt to stdout
		if err != nil {
			view.Diagnostics(diags.Append(fmt.Errorf("Failed to write state: %s", err)))
			return 1
		}

		view.PrintPulledState(buf.String())
	}

	return 0
}

func (c *StatePullCommand) Help() string {
	helpText := `
Usage: tofu [global options] state pull [options]

  Pull the state from its location, upgrade the local copy, and output it
  to stdout.

  This command "pulls" the current state and outputs it to stdout.
  As part of this process, OpenTofu will upgrade the state format of the
  local copy to the current version.

  The primary use of this is for state stored remotely. This command
  will still work with local state but is less useful for this.

Options:

  -var 'foo=bar'     Set a value for one of the input variables in the root
                     module of the configuration. Use this option more than
                     once to set more than one variable.

  -var-file=filename Load variable values from the given file, in addition
                     to the default files terraform.tfvars and *.auto.tfvars.
                     Use this option more than once to include more than one
                     variables file.
`
	return strings.TrimSpace(helpText)
}

// TODO meta-refactor: move this to arguments once all commands are using the same shim logic
func (c *StatePullCommand) GatherVariables(args *arguments.Vars) {
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

func (c *StatePullCommand) Synopsis() string {
	return "Pull current state and output to stdout"
}
