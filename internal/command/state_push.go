// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mitchellh/cli"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/clistate"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

// StatePushCommand is a Command implementation that shows a single resource.
type StatePushCommand struct {
	Meta
	StateMeta
}

func (c *StatePushCommand) Run(args []string) int {
	ctx := c.CommandContext()
	args = c.Meta.process(args)
	var flagForce bool
	cmdFlags := c.Meta.ignoreRemoteVersionFlagSet("state push")
	cmdFlags.BoolVar(&flagForce, "force", false, "")
	cmdFlags.BoolVar(&c.Meta.stateLock, "lock", true, "lock state")
	cmdFlags.DurationVar(&c.Meta.stateLockTimeout, "lock-timeout", 0, "lock timeout")
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line flags: %s\n", err.Error()))
		return 1
	}
	args = cmdFlags.Args()

	if len(args) != 1 {
		c.Ui.Error("Exactly one argument expected.\n")
		return cli.RunResultHelp
	}

	if diags := c.Meta.checkRequiredVersion(ctx); diags != nil {
		c.showDiagnostics(diags)
		return 1
	}

	// Load the encryption configuration
	enc, encDiags := c.Encryption(ctx)
	if encDiags.HasErrors() {
		c.showDiagnostics(encDiags)
		return 1
	}

	// Determine our reader for the input state. This is the filepath
	// or stdin if "-" is given.
	var r io.Reader = os.Stdin
	if args[0] != "-" {
		f, err := os.Open(args[0])
		if err != nil {
			c.Ui.Error(err.Error())
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
		c.Ui.Error(fmt.Sprintf("Error reading source state %q: %s", args[0], err))
		return 1
	}

	// Load the backend
	b, backendDiags := c.Backend(ctx, nil, enc.State())
	if backendDiags.HasErrors() {
		c.showDiagnostics(backendDiags)
		return 1
	}

	// Determine the workspace name
	workspace, err := c.Workspace(ctx)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error selecting workspace: %s", err))
		return 1
	}

	// Check remote OpenTofu version is compatible
	remoteVersionDiags := c.remoteVersionCheck(b, workspace)
	c.showDiagnostics(remoteVersionDiags)
	if remoteVersionDiags.HasErrors() {
		return 1
	}

	// Get the state manager for the currently-selected workspace
	stateMgr, err := b.StateMgr(workspace)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Failed to load destination state: %s", err))
		return 1
	}

	if c.stateLock {
		stateLocker := clistate.NewLocker(c.stateLockTimeout, views.NewStateLocker(arguments.ViewHuman, c.View))
		if diags := stateLocker.Lock(stateMgr, "state-push"); diags.HasErrors() {
			c.showDiagnostics(diags)
			return 1
		}
		defer func() {
			if diags := stateLocker.Unlock(); diags.HasErrors() {
				c.showDiagnostics(diags)
			}
		}()
	}

	if err := stateMgr.RefreshState(); err != nil {
		c.Ui.Error(fmt.Sprintf("Failed to refresh destination state: %s", err))
		return 1
	}

	if srcStateFile == nil {
		// We'll push a new empty state instead
		srcStateFile = statemgr.NewStateFile()
	}

	// Import it, forcing through the lineage/serial if requested and possible.
	if err := statemgr.Import(srcStateFile, stateMgr, flagForce); err != nil {
		c.Ui.Error(fmt.Sprintf("Failed to write state: %s", err))
		return 1
	}

	// Get schemas, if possible, before writing state
	var schemas *tofu.Schemas
	var diags tfdiags.Diagnostics
	if isCloudMode(b) {
		schemas, diags = c.MaybeGetSchemas(ctx, srcStateFile.State, nil)
	}

	if err := stateMgr.WriteState(srcStateFile.State); err != nil {
		c.Ui.Error(fmt.Sprintf("Failed to write state: %s", err))
		return 1
	}
	if err := stateMgr.PersistState(schemas); err != nil {
		c.Ui.Error(fmt.Sprintf("Failed to persist state: %s", err))
		return 1
	}

	c.showDiagnostics(diags)
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

`
	return strings.TrimSpace(helpText)
}

func (c *StatePushCommand) Synopsis() string {
	return "Update remote state from a local state file"
}
