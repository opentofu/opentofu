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

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/clistate"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

// StateRmCommand is a Command implementation that shows a single resource.
type StateRmCommand struct {
	StateMeta
}

func (c *StateRmCommand) Run(args []string) int {
	ctx := c.CommandContext()
	args = c.Meta.process(args)
	var dryRun bool
	cmdFlags := c.Meta.ignoreRemoteVersionFlagSet("state rm")
	cmdFlags.BoolVar(&dryRun, "dry-run", false, "dry run")
	cmdFlags.StringVar(&c.backupPath, "backup", "-", "backup")
	cmdFlags.BoolVar(&c.Meta.stateLock, "lock", true, "lock state")
	cmdFlags.DurationVar(&c.Meta.stateLockTimeout, "lock-timeout", 0, "lock timeout")
	cmdFlags.StringVar(&c.statePath, "state", "", "path")
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line flags: %s\n", err.Error()))
		return 1
	}

	args = cmdFlags.Args()
	if len(args) < 1 {
		c.Ui.Error("At least one address is required.\n")
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

	// Get the state
	stateMgr, err := c.State(ctx, enc)
	if err != nil {
		c.Ui.Error(fmt.Sprintf(errStateLoadingState, err))
		return 1
	}

	if c.stateLock {
		stateLocker := clistate.NewLocker(c.stateLockTimeout, views.NewStateLocker(arguments.ViewHuman, c.View))
		if diags := stateLocker.Lock(stateMgr, "state-rm"); diags.HasErrors() {
			c.showDiagnostics(diags)
			return 1
		}
		defer func() {
			if diags := stateLocker.Unlock(); diags.HasErrors() {
				c.showDiagnostics(diags)
			}
		}()
	}

	if err := stateMgr.RefreshState(context.TODO()); err != nil {
		c.Ui.Error(fmt.Sprintf("Failed to refresh state: %s", err))
		return 1
	}

	state := stateMgr.State()
	if state == nil {
		c.Ui.Error(errStateNotFound)
		return 1
	}

	// This command primarily works with resource instances, though it will
	// also clean up any modules and resources left empty by actions it takes.
	var addrs []addrs.AbsResourceInstance
	var diags tfdiags.Diagnostics
	for _, addrStr := range args {
		moreAddrs, moreDiags := c.lookupResourceInstanceAddr(state, true, addrStr)
		addrs = append(addrs, moreAddrs...)
		diags = diags.Append(moreDiags)
	}
	if diags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	prefix := "Removed "
	if dryRun {
		prefix = "Would remove "
	}

	var isCount int
	ss := state.SyncWrapper()
	for _, addr := range addrs {
		isCount++
		c.Ui.Output(prefix + addr.String())
		if !dryRun {
			ss.ForgetResourceInstanceAll(addr)
			ss.RemoveResourceIfEmpty(addr.ContainingResource())
		}
	}

	if dryRun {
		if isCount == 0 {
			c.Ui.Output("Would have removed nothing.")
		}
		return 0 // This is as far as we go in dry-run mode
	}

	b, backendDiags := c.Backend(ctx, nil, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	// Get schemas, if possible, before writing state
	var schemas *tofu.Schemas
	if isCloudMode(b) {
		var schemaDiags tfdiags.Diagnostics
		schemas, schemaDiags = c.MaybeGetSchemas(ctx, state, nil)
		diags = diags.Append(schemaDiags)
	}

	if err := stateMgr.WriteState(state); err != nil {
		c.Ui.Error(fmt.Sprintf(errStateRmPersist, err))
		return 1
	}
	if err := stateMgr.PersistState(context.TODO(), schemas); err != nil {
		c.Ui.Error(fmt.Sprintf(errStateRmPersist, err))
		return 1
	}

	if len(diags) > 0 && isCount != 0 {
		c.showDiagnostics(diags)
	}

	if isCount == 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid target address",
			"No matching objects found. To view the available instances, use \"tofu state list\". Please modify the address to reference a specific instance.",
		))
		c.showDiagnostics(diags)
		return 1
	}

	c.Ui.Output(fmt.Sprintf("Successfully removed %d resource instance(s).", isCount))
	return 0
}

func (c *StateRmCommand) Help() string {
	helpText := `
Usage: tofu [global options] state (remove|rm) [options] ADDRESS...

  Remove one or more items from the OpenTofu state, causing OpenTofu to
  "forget" those items without first destroying them in the remote system.

  This command removes one or more resource instances from the OpenTofu state
  based on the addresses given. You can view and list the available instances
  with "tofu state list".

  If you give the address of an entire module then all of the instances in
  that module and any of its child modules will be removed from the state.

  If you give the address of a resource that has "count" or "for_each" set,
  all of the instances of that resource will be removed from the state.

Options:

  -dry-run                If set, prints out what would've been removed but
                          doesn't actually remove anything.

  -backup=PATH            Path where OpenTofu should write the backup
                          state.

  -lock=false             Don't hold a state lock during the operation. This is
                          dangerous if others might concurrently run commands
                          against the same workspace.

  -lock-timeout=0s        Duration to retry a state lock.

  -state=PATH             Path to the state file to update. Defaults to the
                          current workspace state.

  -ignore-remote-version  Continue even if remote and local OpenTofu versions
                          are incompatible. This may result in an unusable
                          workspace, and should be used with extreme caution.

  -var 'foo=bar'          Set a value for one of the input variables in the root
                          module of the configuration. Use this option more than
                          once to set more than one variable.

  -var-file=filename      Load variable values from the given file, in addition
                          to the default files terraform.tfvars and *.auto.tfvars.
                          Use this option more than once to include more than one
                          variables file.

`
	return strings.TrimSpace(helpText)
}

func (c *StateRmCommand) Synopsis() string {
	return "Remove instances from the state"
}

const errStateRmPersist = `Error saving the state: %s

The state was not saved. No items were removed from the persisted
state. No backup was created since no modification occurred. Please
resolve the issue above and try again.`
