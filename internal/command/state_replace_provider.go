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

func (c *StateReplaceProviderCommand) Run(args []string) int {
	ctx := c.CommandContext()

	args = c.Meta.process(args)

	var autoApprove bool
	cmdFlags := c.Meta.ignoreRemoteVersionFlagSet("state replace-provider")
	cmdFlags.BoolVar(&autoApprove, "auto-approve", false, "skip interactive approval of replacements")
	cmdFlags.StringVar(&c.backupPath, "backup", "-", "backup")
	cmdFlags.BoolVar(&c.Meta.stateLock, "lock", true, "lock states")
	cmdFlags.DurationVar(&c.Meta.stateLockTimeout, "lock-timeout", 0, "lock timeout")
	cmdFlags.StringVar(&c.statePath, "state", "", "path")
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line flags: %s\n", err.Error()))
		return cli.RunResultHelp
	}
	args = cmdFlags.Args()
	if len(args) != 2 {
		c.Ui.Error("Exactly two arguments expected.\n")
		return cli.RunResultHelp
	}

	if diags := c.Meta.checkRequiredVersion(ctx); diags != nil {
		c.showDiagnostics(diags)
		return 1
	}

	var diags tfdiags.Diagnostics

	// Parse from/to arguments into providers
	from, fromDiags := addrs.ParseProviderSourceString(args[0])
	if fromDiags.HasErrors() {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			fmt.Sprintf(`Invalid "from" provider %q`, args[0]),
			fromDiags.Err().Error(),
		))
	}
	to, toDiags := addrs.ParseProviderSourceString(args[1])
	if toDiags.HasErrors() {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			fmt.Sprintf(`Invalid "to" provider %q`, args[1]),
			toDiags.Err().Error(),
		))
	}
	if diags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	// Load the encryption configuration
	enc, encDiags := c.Encryption(ctx)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	// Initialize the state manager as configured
	stateMgr, err := c.State(ctx, enc)
	if err != nil {
		c.Ui.Error(fmt.Sprintf(errStateLoadingState, err))
		return 1
	}

	// Acquire lock if requested
	if c.stateLock {
		stateLocker := clistate.NewLocker(c.stateLockTimeout, views.NewStateLocker(arguments.ViewOptions{ViewType: arguments.ViewHuman}, c.View))
		if diags := stateLocker.Lock(stateMgr, "state-replace-provider"); diags.HasErrors() {
			c.showDiagnostics(diags)
			return 1
		}
		defer func() {
			if diags := stateLocker.Unlock(); diags.HasErrors() {
				c.showDiagnostics(diags)
			}
		}()
	}

	// Refresh and load state
	if err := stateMgr.RefreshState(context.TODO()); err != nil {
		c.Ui.Error(fmt.Sprintf("Failed to refresh source state: %s", err))
		return 1
	}

	state := stateMgr.State()
	if state == nil {
		c.Ui.Error(errStateNotFound)
		return 1
	}

	// Fetch all resources from the state
	resources, diags := c.lookupAllResources(state)
	if diags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	var willReplace []*states.Resource

	// Update all matching resources with new provider
	for _, resource := range resources {
		if resource.ProviderConfig.Provider.Equals(from) {
			willReplace = append(willReplace, resource)
		}
	}
	c.showDiagnostics(diags)

	if len(willReplace) == 0 {
		c.Ui.Output("No matching resources found.")
		return 0
	}

	// Explain the changes
	colorize := c.Colorize()
	c.Ui.Output("OpenTofu will perform the following actions:\n")
	c.Ui.Output(colorize.Color("  [yellow]~[reset] Updating provider:"))
	c.Ui.Output(colorize.Color(fmt.Sprintf("    [red]-[reset] %s", from)))
	c.Ui.Output(colorize.Color(fmt.Sprintf("    [green]+[reset] %s\n", to)))

	c.Ui.Output(colorize.Color(fmt.Sprintf("[bold]Changing[reset] %d resources:\n", len(willReplace))))
	for _, resource := range willReplace {
		c.Ui.Output(colorize.Color(fmt.Sprintf("  %s", resource.Addr)))
	}

	// Confirm
	if !autoApprove {
		c.Ui.Output(colorize.Color(
			"\n[bold]Do you want to make these changes?[reset]\n" +
				"Only 'yes' will be accepted to continue.\n",
		))
		v, err := c.Ui.Ask("Enter a value:")
		if err != nil {
			c.Ui.Error(fmt.Sprintf("Error asking for approval: %s", err))
			return 1
		}
		if v != "yes" {
			c.Ui.Output("Cancelled replacing providers.")
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

	// Write the updated state
	if err := stateMgr.WriteState(state); err != nil {
		c.Ui.Error(fmt.Sprintf(errStateRmPersist, err))
		return 1
	}
	if err := stateMgr.PersistState(context.TODO(), schemas); err != nil {
		c.Ui.Error(fmt.Sprintf(errStateRmPersist, err))
		return 1
	}

	c.showDiagnostics(diags)
	c.Ui.Output(fmt.Sprintf("\nSuccessfully replaced provider for %d resources.", len(willReplace)))
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

  -state, state-out, and -backup are legacy options supported for the local
  backend only. For more information, see the local backend's documentation.

`
	return strings.TrimSpace(helpText)
}

func (c *StateReplaceProviderCommand) Synopsis() string {
	return "Replace provider in the state"
}
