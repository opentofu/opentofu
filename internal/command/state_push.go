// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/flags"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/clistate"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/internal/tofu"
)

// StatePushCommand is a Command implementation that shows a single resource.
type StatePushCommand struct {
	Meta
	StateMeta
}

func (c *StatePushCommand) Run(rawArgs []string) int {
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
	args, closer, diags := arguments.ParseStatePush(rawArgs)
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
	// TODO meta-refactor: remove these assignments once we have a clear way to propagate these to the logic
	//  that uses them
	c.GatherVariables(args.Vars)
	c.stateLock = args.Backend.StateLock
	c.stateLockTimeout = args.Backend.StateLockTimeout
	c.ignoreRemoteVersion = args.Backend.IgnoreRemoteVersion

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

	// Determine our reader for the input state. This is the filepath
	// or stdin if "-" is given.
	var r io.Reader = os.Stdin
	if src := args.StateSrc; src != "-" {
		f, err := os.Open(src)
		if err != nil {
			view.Diagnostics(diags.Append(err))
			return 1
		}
		// Note: we don't need to defer a Close here because we do a close
		// automatically below directly after the read.
		r = f
	}

	// Read the state
	srcStateFile, err := statefile.Read(r, encryption.StateEncryptionDisabled()) // Assume the given statefile is not encrypted
	if c, ok := r.(io.Closer); ok {
		// Close the reader if possible right now since we're done with it.
		c.Close()
	}
	if err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Error reading source state %q: %s", args.StateSrc, err)))
		return 1
	}

	// Load the backend
	b, backendDiags := c.Backend(ctx, nil, enc.State())
	if backendDiags.HasErrors() {
		view.Diagnostics(backendDiags)
		return 1
	}

	// Determine the workspace name
	workspace, err := c.Workspace(ctx)
	if err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Error selecting workspace: %s", err)))
		return 1
	}

	// Check remote OpenTofu version is compatible
	remoteVersionDiags := c.remoteVersionCheck(b, workspace)
	view.Diagnostics(remoteVersionDiags)
	if remoteVersionDiags.HasErrors() {
		return 1
	}

	// Get the state manager for the currently-selected workspace
	stateMgr, err := b.StateMgr(ctx, workspace)
	if err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Failed to load destination state: %s", err)))
		return 1
	}

	if c.stateLock {
		stateLocker := clistate.NewLocker(c.stateLockTimeout, views.NewStateLocker(args.ViewOptions, c.View))
		if diags := stateLocker.Lock(stateMgr, "state-push"); diags.HasErrors() {
			view.Diagnostics(diags)
			return 1
		}
		defer func() {
			if diags := stateLocker.Unlock(); diags.HasErrors() {
				view.Diagnostics(diags)
			}
		}()
	}

	if err := stateMgr.RefreshState(context.TODO()); err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Failed to refresh destination state: %s", err)))
		return 1
	}

	if srcStateFile == nil {
		// We'll push a new empty state instead
		srcStateFile = statemgr.NewStateFile()
	}

	// Import it, forcing through the lineage/serial if requested and possible.
	if err := statemgr.Import(srcStateFile, stateMgr, args.Force); err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Failed to write state: %s", err)))
		return 1
	}

	// Get schemas, if possible, before writing state
	var schemas *tofu.Schemas
	if isCloudMode(b) {
		schemas, diags = c.MaybeGetSchemas(ctx, srcStateFile.State, nil)
	}

	if err := stateMgr.WriteState(srcStateFile.State); err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Failed to write state: %s", err)))
		return 1
	}
	if err := stateMgr.PersistState(context.TODO(), schemas); err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Failed to persist state: %s", err)))
		return 1
	}

	view.Diagnostics(diags)
	return 0
}

func (c *StatePushCommand) Help() string {
	helpText := `
Usage: tofu [global options] state push [options] PATH

  Update remote state from a local state file at PATH.

  This command "pushes" a local state and overwrites remote state
  with a local state file. The command will protect you against writing
  an older serial or a different state file lineage unless you specify the
  "-force" flag.

  This command works with local state (it will overwrite the local
  state), but is less useful for this use case.

  If PATH is "-", then this command will read the state to push from stdin.
  Data from stdin is not streamed to the backend: it is loaded completely
  (until pipe close), verified, and then pushed.

Options:

  -force              Write the state even if lineages don't match or the
                      remote serial is higher.

  -lock=false         Don't hold a state lock during the operation. This is
                      dangerous if others might concurrently run commands
                      against the same workspace.

  -lock-timeout=0s    Duration to retry a state lock.

  -var 'foo=bar'      Set a value for one of the input variables in the root
                      module of the configuration. Use this option more than
                      once to set more than one variable.

  -var-file=filename  Load variable values from the given file, in addition
                      to the default files terraform.tfvars and *.auto.tfvars.
                      Use this option more than once to include more than one
                      variables file.

  -json               Produce output in a machine-readable JSON format, 
                      suitable for use in text editor integrations and other 
                      automated systems. Always disables color.

  -json-into=out.json Produce the same output as -json, but sent directly
                      to the given file. This allows automation to preserve
                      the original human-readable output streams, while
                      capturing more detailed logs for machine analysis.

`
	return strings.TrimSpace(helpText)
}

func (c *StatePushCommand) Synopsis() string {
	return "Update remote state from a local state file"
}

// TODO meta-refactor: move this to arguments once all commands are using the same shim logic
func (c *StatePushCommand) GatherVariables(args *arguments.Vars) {
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
