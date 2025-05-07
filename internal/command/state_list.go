// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"strings"

	"github.com/mitchellh/cli"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// StateListCommand is a Command implementation that lists the resources
// within a state file.
type StateListCommand struct {
	Meta
	StateMeta
}

func (c *StateListCommand) Run(args []string) int {
	ctx := c.CommandContext()

	args = c.Meta.process(args)
	var statePath string
	cmdFlags := c.Meta.defaultFlagSet("state list")
	c.Meta.varFlagSet(cmdFlags)
	cmdFlags.StringVar(&statePath, "state", "", "path")
	lookupId := cmdFlags.String("id", "", "Restrict output to paths with a resource having the specified ID.")
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line flags: %s\n", err.Error()))
		return cli.RunResultHelp
	}
	args = cmdFlags.Args()

	if statePath != "" {
		c.Meta.statePath = statePath
	}

	// Load the encryption configuration
	enc, encDiags := c.Encryption(ctx)
	if encDiags.HasErrors() {
		c.showDiagnostics(encDiags)
		return 1
	}

	// Load the backend
	b, backendDiags := c.Backend(ctx, nil, enc.State())
	if backendDiags.HasErrors() {
		c.showDiagnostics(backendDiags)
		return 1
	}

	// This is a read-only command
	c.ignoreRemoteVersionConflict(b)

	// Get the state
	env, err := c.Workspace(ctx)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error selecting workspace: %s", err))
		return 1
	}
	stateMgr, err := b.StateMgr(ctx, env)
	if err != nil {
		c.Ui.Error(fmt.Sprintf(errStateLoadingState, err))
		return 1
	}
	if err := stateMgr.RefreshState(); err != nil {
		c.Ui.Error(fmt.Sprintf("Failed to load state: %s", err))
		return 1
	}

	state := stateMgr.State()
	if state == nil {
		c.Ui.Error(errStateNotFound)
		return 1
	}

	var addrs []addrs.AbsResourceInstance
	var diags tfdiags.Diagnostics
	if len(args) == 0 {
		addrs, diags = c.lookupAllResourceInstanceAddrs(state)
	} else {
		addrs, diags = c.lookupResourceInstanceAddrs(state, args...)
	}
	if diags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	for _, addr := range addrs {
		if is := state.ResourceInstance(addr); is != nil {
			if *lookupId == "" || *lookupId == states.LegacyInstanceObjectID(is.Current) {
				c.Ui.Output(addr.String())
			}
		}
	}

	c.showDiagnostics(diags)

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

`
	return strings.TrimSpace(helpText)
}

func (c *StateListCommand) Synopsis() string {
	return "List resources in the state"
}

const errStateLoadingState = `Error loading the state: %[1]s

Please ensure that your OpenTofu state exists and that you've
configured it properly. You can use the "-state" flag to point
OpenTofu at another state file.`

const errStateNotFound = `No state file was found!

State management commands require a state file. Run this command
in a directory where OpenTofu has been run or use the -state flag
to point the command to a specific state location.`
