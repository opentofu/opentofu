// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/internal/tracing"

	"github.com/mitchellh/cli"

	"github.com/opentofu/opentofu/internal/tofu"
)

// UnlockCommand is a cli.Command implementation that manually unlocks
// the state.
type UnlockCommand struct {
	Meta
}

func (c *UnlockCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()
	ctx, span := tracing.Tracer().Start(ctx, "Unlock")
	defer span.End()

	// new view
	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)
	// Because the legacy UI was using println to show diagnostics and the new view is using, by default, print,
	// in order to keep functional parity, we setup the view to add a new line after each diagnostic.
	c.View.DiagsWithNewline()

	// Parse and validate flags
	args, closer, diags := arguments.ParseUnlock(rawArgs)
	defer closer()

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewUnlock(args.ViewOptions, c.View)
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
	c.Meta.variableArgs = args.Vars.All()

	lockID := args.LockID

	// This gets the current directory as full path.
	configPath := c.WorkingDir.NormalizePath(c.WorkingDir.RootModuleDir())

	// Load the encryption configuration
	enc, encDiags := c.EncryptionFromPath(ctx, configPath)
	if encDiags.HasErrors() {
		view.Diagnostics(encDiags)
		return 1
	}

	backendConfig, backendDiags := c.loadBackendConfig(ctx, configPath)
	diags = diags.Append(backendDiags)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Load the backend
	b, backendDiags := c.Backend(ctx, &BackendOpts{
		Config: backendConfig,
		View:   view.Backend(),
	}, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// unlocking is read only when looking at state data
	c.ignoreRemoteVersionConflict(b)

	env, err := c.Workspace(ctx)
	if err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Error selecting workspace: %s", err)))
		return 1
	}
	stateMgr, err := b.StateMgr(ctx, env)
	if err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Failed to load state: %s", err)))
		return 1
	}

	_, isLocal := stateMgr.(*statemgr.Filesystem)

	if optionalLocker, ok := stateMgr.(statemgr.OptionalLocker); ok {
		// Now we can safely call IsLockingEnabled() on optionalLocker
		if !optionalLocker.IsLockingEnabled() {
			view.LockingDisabledForBackend()
			return 1
		}
	}

	// Proceed with unlocking logic if locking is enabled
	if !args.Force {
		// Forcing this doesn't do anything, but doesn't break anything either,
		// and allows us to run the basic command test too.
		if isLocal {
			view.CannotUnlockByAnotherProcess()
			return 1
		}

		desc := "OpenTofu will remove the lock on the remote state.\n" +
			"This will allow local OpenTofu commands to modify this state, even though it\n" +
			"may still be in use. Only 'yes' will be accepted to confirm."

		v, err := c.UIInput().Input(context.Background(), &tofu.InputOpts{
			Id:          "force-unlock",
			Query:       "Do you really want to force-unlock?",
			Description: desc,
		})
		if err != nil {
			view.Diagnostics(diags.Append(fmt.Errorf("Error asking for confirmation: %s", err)))
			return 1
		}
		if v != "yes" {
			view.ForceUnlockCancelled()
			return 1
		}
	}

	if err := stateMgr.Unlock(context.TODO(), lockID); err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Failed to unlock state: %s", err)))
		return 1
	}
	view.ForceUnlockSucceeded()
	return 0
}

func (c *UnlockCommand) Help() string {
	helpText := `
Usage: tofu [global options] force-unlock [options] LOCK_ID

  Manually unlock the state for the defined configuration.

  This will not modify your infrastructure. This command removes the lock on the
  state for the current workspace. The behavior of this lock is dependent
  on the backend being used. Local state files cannot be unlocked by another
  process.

Options:

  -force                 Don't ask for input for unlock confirmation.

  -var 'foo=bar'         Set a value for one of the input variables in the root
                         module of the configuration. Use this option more than
                         once to set more than one variable.

  -var-file=filename     Load variable values from the given file, in addition
                         to the default files terraform.tfvars and *.auto.tfvars.
                         Use this option more than once to include more than one
                         variables file.

  -json                  Produce output in a machine-readable JSON format, 
                         suitable for use in text editor integrations and other 
                         automated systems. Always disables color.

  -json-into=out.json    Produce the same output as -json, but sent directly
                         to the given file. This allows automation to preserve
                         the original human-readable output streams, while
                         capturing more detailed logs for machine analysis.
`
	return strings.TrimSpace(helpText)
}

func (c *UnlockCommand) Synopsis() string {
	return "Release a stuck lock on the current workspace"
}
