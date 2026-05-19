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
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/tfdiags"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states"
)

// StateListCommand is a Command implementation that lists the resources
// within a state file.
type StateListCommand struct {
	Meta
	StateMeta
}

func (c *StateListCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()

	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)

	// Parse and validate flags
	args, closer, diags := arguments.ParseStateList(rawArgs)
	defer closer()

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewState(args.ViewOptions, c.View)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		if args.ViewOptions.ViewType == arguments.ViewJSON {
			return 1 // in case it's json, do not print the help of the command
		}
		return cli.RunResultHelp
	}
	c.Meta.variableArgs = args.Vars.All()

	if args.State.StatePath != "" {
		c.Meta.stateArgs.StatePath = args.State.StatePath
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

	// Get the state
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
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error refreshing the state",
			fmt.Sprintf("Failed to load state: %s", err),
		)))
		return 1
	}

	state := stateMgr.State()
	if state == nil {
		view.StateNotFound()
		return 1
	}

	var resourceAddrs []addrs.AbsResourceInstance
	if len(args.InstancesRawAddr) == 0 {
		resourceAddrs, diags = c.lookupAllResourceInstanceAddrs(state)
	} else {
		resourceAddrs, diags = c.lookupResourceInstanceAddrs(state, args.InstancesRawAddr...)
	}
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	for _, addr := range resourceAddrs {
		if is := state.ResourceInstance(addr); is != nil {
			if args.LookupId == "" || args.LookupId == states.LegacyInstanceObjectID(is.Current) {
				view.StateListAddr(addr)
			}
		}
	}

	view.Diagnostics(diags)

	return 0
}

func (c *StateListCommand) Help() string {
	helpText := `
Usage: tofu [global options] state (list|ls) [options] [address...]

  List resources in the OpenTofu state.

  This command lists resource instances in the OpenTofu state. The address
  argument can be used to filter the instances by resource or module. If
  no pattern is given, all resource instances are listed.

  The addresses must either be module addresses or absolute resource
  addresses, such as:
      aws_instance.example
      module.example
      module.example.module.child
      module.example.aws_instance.example

  An error will be returned if any of the resources or modules given as
  filter addresses do not exist in the state.

Options:

  -state=statefile    Path to a OpenTofu state file to use to look
                      up OpenTofu-managed resources. By default, OpenTofu
                      will consult the state of the currently-selected
                      workspace.

  -id=ID              Filters the results to include only instances whose
                      resource types have an attribute named "id" whose value
                      equals the given id string.

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

func (c *StateListCommand) Synopsis() string {
	return "List resources in the state"
}
