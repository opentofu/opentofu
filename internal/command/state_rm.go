// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
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

func (c *StateRmCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()

	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)
	// Because the legacy UI was using println to show diagnostics and the new view is using, by default, print,
	// in order to keep functional parity, we setup the view to add a new line after each diagnostic.
	c.View.DiagsWithNewline()

	// Parse and validate flags
	args, closer, diags := arguments.ParseStateRm(rawArgs)
	defer closer()

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewState(args.ViewOptions, c.View)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		if args.ViewOptions.ViewType == arguments.ViewJSON {
			return 1 // We don't want to print the help of the command in JSON view
		}
		return cli.RunResultHelp
	}
	c.Meta.variableArgs = args.Vars.All()
	c.stateArgs = *args.State
	c.backendArgs = *args.Backend

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

	// Get the state
	stateMgr, err := c.State(ctx, enc, view)
	if err != nil {
		view.StateLoadingFailure(err.Error())
		return 1
	}

	if c.stateArgs.Lock {
		stateLocker := clistate.NewLocker(c.stateArgs.LockTimeout, view.Backend().StateLocker())
		if diags := stateLocker.Lock(stateMgr, "state-rm"); diags.HasErrors() {
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
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to refresh state",
			err.Error(),
		)))
		return 1
	}

	state := stateMgr.State()
	if state == nil {
		view.StateNotFound()
		return 1
	}

	// This command primarily works with resource instances, though it will
	// also clean up any modules and resources left empty by actions it takes.
	var resAddrs []addrs.AbsResourceInstance
	for _, addrStr := range args.TargetAddrs {
		moreAddrs, moreDiags := c.lookupResourceInstanceAddr(state, true, addrStr)
		resAddrs = append(resAddrs, moreAddrs...)
		diags = diags.Append(moreDiags)
	}
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	var isCount int
	ss := state.SyncWrapper()
	for _, addr := range resAddrs {
		isCount++
		view.ResourceRemoveStatus(args.DryRun, addr.String())
		if !args.DryRun {
			ss.ForgetResourceInstanceAll(addr)
			ss.RemoveResourceIfEmpty(addr.ContainingResource())
		}
	}

	if args.DryRun {
		view.DryRunRemovedStatus(isCount)
		return 0 // This is as far as we go in dry-run mode
	}

	b, backendDiags := c.Backend(ctx, nil, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		view.Diagnostics(diags)
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
		view.StateSavingError(err.Error())
		return 1
	}
	if err := stateMgr.PersistState(context.TODO(), schemas); err != nil {
		view.StateSavingError(err.Error())
		return 1
	}

	if len(diags) > 0 && isCount != 0 {
		view.Diagnostics(diags)
	}

	if isCount == 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid target address",
			"No matching objects found. To view the available instances, use \"tofu state list\". Please modify the address to reference a specific instance.",
		))
		view.Diagnostics(diags)
		return 1
	}

	view.RemoveFinalStatus(isCount)
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

  -json                   Produce output in a machine-readable JSON format, 
                          suitable for use in text editor integrations and other 
                          automated systems. Always disables color.

  -json-into=out.json     Produce the same output as -json, but sent directly
                          to the given file. This allows automation to preserve
                          the original human-readable output streams, while
                          capturing more detailed logs for machine analysis.

`
	return strings.TrimSpace(helpText)
}

func (c *StateRmCommand) Synopsis() string {
	return "Remove instances from the state"
}
