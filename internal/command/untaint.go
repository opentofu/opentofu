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
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

// UntaintCommand is a cli.Command implementation that manually untaints
// a resource, marking it as primary and ready for service.
type UntaintCommand struct {
	Meta
}

func (c *UntaintCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()

	// new view
	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)
	// Because the legacy UI was using println to show diagnostics and the new view is using, by default, print,
	// in order to keep functional parity, we setup the view to add a new line after each diagnostic.
	c.View.DiagsWithNewline()

	// Parse and validate flags
	args, closer, diags := arguments.ParseTaint(false, rawArgs)
	defer closer()

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewTaint(args.ViewOptions, c.View)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		if args.ViewOptions.ViewType == arguments.ViewJSON {
			return 1 // in case it's json, do not print the help of the command
		}
		return cli.RunResultHelp
	}
	c.Meta.variableArgs = args.Vars.All()
	c.Meta.stateArgs = *args.State

	addr := args.TargetAddress

	// Load the encryption configuration
	enc, encDiags := c.Encryption(ctx)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Load the backend
	b, backendDiags := c.Backend(ctx, nil, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Determine the workspace name
	workspace, err := c.Workspace(ctx)
	if err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error selecting the current workspace name",
			fmt.Sprintf("Failed to select the current workspace: %s", err),
		)))
		return 1
	}

	// Check remote OpenTofu version is compatible
	remoteVersionDiags := c.remoteVersionCheck(b, workspace)
	diags = diags.Append(remoteVersionDiags)
	view.Diagnostics(diags)
	if diags.HasErrors() {
		return 1
	}
	// since we already printed the diagnostics above, we can discard any possible warnings
	diags = tfdiags.Diagnostics{}

	// Get the state
	stateMgr, err := b.StateMgr(ctx, workspace)
	if err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error loading the state",
			fmt.Sprintf("Failed to load the state: %s", err),
		)))
		return 1
	}

	if c.stateArgs.Lock {
		stateLocker := clistate.NewLocker(c.stateArgs.LockTimeout, view.Backend().StateLocker())
		if diags := stateLocker.Lock(stateMgr, "untaint"); diags.HasErrors() {
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
			"Error refreshing the state",
			fmt.Sprintf("Failed to load state: %s", err),
		)))
		return 1
	}

	// Get the actual state structure
	state := stateMgr.State()
	if state.Empty() {
		if args.AllowMissing {
			return c.allowMissingExit(addr, view)
		}

		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"No such resource instance",
			"The state currently contains no resource instances whatsoever. This may occur if the configuration has never been applied or if it has recently been destroyed.",
		))
		view.Diagnostics(diags)
		return 1
	}

	ss := state.SyncWrapper()

	// Get the resource and instance we're going to taint
	rs := ss.Resource(addr.ContainingResource())
	is := ss.ResourceInstance(addr)
	if is == nil {
		if args.AllowMissing {
			return c.allowMissingExit(addr, view)
		}

		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"No such resource instance",
			fmt.Sprintf("There is no resource instance in the state with the address %s. If the resource configuration has just been added, you must run \"tofu apply\" once to create the corresponding instance(s) before they can be tainted.", addr),
		))
		view.Diagnostics(diags)
		return 1
	}

	obj := is.Current
	if obj == nil {
		if len(is.Deposed) != 0 {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"No such resource instance",
				fmt.Sprintf("Resource instance %s is currently part-way through a create_before_destroy replacement action. Run \"tofu apply\" to complete its replacement before tainting it.", addr),
			))
		} else {
			// Don't know why we're here, but we'll produce a generic error message anyway.
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"No such resource instance",
				fmt.Sprintf("Resource instance %s does not currently have a remote object associated with it, so it cannot be tainted.", addr),
			))
		}
		view.Diagnostics(diags)
		return 1
	}

	if obj.Status != states.ObjectTainted {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Resource instance is not tainted",
			fmt.Sprintf("Resource instance %s is not currently tainted, and so it cannot be untainted.", addr),
		))
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

	obj.Status = states.ObjectReady
	ss.SetResourceInstanceCurrent(addr, obj, rs.ProviderConfig, is.ProviderKey)

	if err := stateMgr.WriteState(state); err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error writing state file",
			fmt.Sprintf("Failed writing the new state: %s", err),
		)))
		return 1
	}
	if err := stateMgr.PersistState(context.TODO(), schemas); err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error writing state file",
			fmt.Sprintf("Failed to persist the new state: %s", err),
		)))
		return 1
	}

	view.Diagnostics(diags)
	view.UntaintedSuccessfully(addr)
	return 0
}

func (c *UntaintCommand) Help() string {
	helpText := `
Usage: tofu [global options] untaint [options] name

  OpenTofu uses the term "tainted" to describe a resource instance
  which may not be fully functional, either because its creation
  partially failed or because you've manually marked it as such using
  the "tofu taint" command.

  This command removes that state from a resource instance, causing
  OpenTofu to see it as fully-functional and not in need of
  replacement.

  This will not modify your infrastructure directly. It only avoids
  OpenTofu planning to replace a tainted instance in a future operation.

Options:

  -allow-missing          If specified, the command will succeed (exit code 0)
                          even if the resource is missing.

  -lock=false             Don't hold a state lock during the operation. This is
                          dangerous if others might concurrently run commands
                          against the same workspace.

  -lock-timeout=0s        Duration to retry a state lock.

  -ignore-remote-version  A rare option used for the remote backend only. See
                          the remote backend documentation for more information.

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

  -state, state-out, and -backup are legacy options supported for the local
  backend only. For more information, see the local backend's documentation.

`
	return strings.TrimSpace(helpText)
}

func (c *UntaintCommand) Synopsis() string {
	return "Remove the 'tainted' state from a resource instance"
}

func (c *UntaintCommand) allowMissingExit(name addrs.AbsResourceInstance, view views.Taint) int {
	view.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
		tfdiags.Warning,
		"No such resource instance",
		fmt.Sprintf("Resource instance %s was not found, but this is not an error because -allow-missing was set.", name),
	)})
	return 0
}
