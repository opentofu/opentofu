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

// StateReplaceProviderCommand is a Command implementation that allows users
// to change the provider associated with existing resources. This is only
// likely to be useful if a provider is forked or changes its fully-qualified
// name.
type StateReplaceProviderCommand struct {
	StateMeta
}

func (c *StateReplaceProviderCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()

	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)
	// Because the legacy UI was using println to show diagnostics and the new view is using, by default, print,
	// in order to keep functional parity, we setup the view to add a new line after each diagnostic.
	c.View.DiagsWithNewline()

	// Parse and validate flags
	args, closer, diags := arguments.ParseReplaceProvider(rawArgs)
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
	// TODO meta-refactor: remove these assignments once there is a clear way to propagate these to the place
	//   where are used
	c.backupPath = args.State.BackupPath
	c.statePath = args.State.StatePath
	c.stateLock = args.State.Lock
	c.stateLockTimeout = args.State.LockTimeout
	c.ignoreRemoteVersion = args.Backend.IgnoreRemoteVersion
	c.Meta.variableArgs = args.Vars.All()

	if diags := c.Meta.checkRequiredVersion(ctx); diags != nil {
		view.Diagnostics(diags)
		return 1
	}

	// Parse from/to arguments into providers
	from, fromDiags := addrs.ParseProviderSourceString(args.RawSrcAddr)
	if fromDiags.HasErrors() {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			fmt.Sprintf(`Invalid "from" provider %q`, args.RawSrcAddr),
			fromDiags.Err().Error(),
		))
	}
	to, toDiags := addrs.ParseProviderSourceString(args.RawDestAddr)
	if toDiags.HasErrors() {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			fmt.Sprintf(`Invalid "to" provider %q`, args.RawDestAddr),
			toDiags.Err().Error(),
		))
	}
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Load the encryption configuration
	enc, encDiags := c.Encryption(ctx)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Initialize the state manager as configured
	stateMgr, err := c.State(ctx, enc, view)
	if err != nil {
		view.StateLoadingFailure(err.Error())
		return 1
	}

	// Acquire lock if requested
	if c.stateLock {
		stateLocker := clistate.NewLocker(c.stateLockTimeout, view.Backend().StateLocker())
		if diags := stateLocker.Lock(stateMgr, "state-replace-provider"); diags.HasErrors() {
			view.Diagnostics(diags)
			return 1
		}
		defer func() {
			if diags := stateLocker.Unlock(); diags.HasErrors() {
				view.Diagnostics(diags)
			}
		}()
	}

	// Refresh and load state
	if err := stateMgr.RefreshState(context.TODO()); err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to refresh source state",
			err.Error(),
		)))
		return 1
	}

	state := stateMgr.State()
	if state == nil {
		view.StateNotFound()
		return 1
	}

	// Fetch all resources from the state
	resources, diags := c.lookupAllResources(state)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	var willReplace []*states.Resource

	// Update all matching resources with new provider
	for _, resource := range resources {
		if resource.ProviderConfig.Provider.Equals(from) {
			willReplace = append(willReplace, resource)
		}
	}
	view.Diagnostics(diags)

	if len(willReplace) == 0 {
		view.NoMatchingResourcesForProviderReplacement()
		return 0
	}

	// Explain the changes
	view.ReplaceProviderOverview(from, to, willReplace)

	// Confirm
	if !args.AutoApprove {
		v, err := c.UIInput().Input(ctx, &tofu.InputOpts{
			Id:          "confirm",
			Query:       "\nDo you want to make these changes?",
			Description: "Only 'yes' will be accepted to continue.",
		})
		if err != nil {
			view.Diagnostics(diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Error asking for approval",
				err.Error(),
			)))
			return 1
		}
		if v != "yes" {
			view.ReplaceProviderCancelled()
			return 0
		}
	}

	// Update the provider for each resource
	for _, resource := range willReplace {
		resource.ProviderConfig.Provider = to
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

	// Write the updated state
	if err := stateMgr.WriteState(state); err != nil {
		view.StateSavingError(err.Error())
		return 1
	}
	if err := stateMgr.PersistState(context.TODO(), schemas); err != nil {
		view.StateSavingError(err.Error())
		return 1
	}

	view.Diagnostics(diags)
	view.ProviderReplaced(len(willReplace))
	return 0
}

func (c *StateReplaceProviderCommand) Help() string {
	helpText := `
Usage: tofu [global options] state replace-provider [options] FROM_PROVIDER_FQN TO_PROVIDER_FQN

  Replace provider for resources in the OpenTofu state.

Options:

  -auto-approve           Skip interactive approval.

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

func (c *StateReplaceProviderCommand) Synopsis() string {
	return "Replace provider in the state"
}
