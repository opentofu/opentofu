// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"os"
	"strings"

	"github.com/mitchellh/cli"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/jsonformat"
	"github.com/opentofu/opentofu/internal/command/jsonprovider"
	"github.com/opentofu/opentofu/internal/command/jsonstate"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofumigrate"
)

// StateShowCommand is a Command implementation that shows a single resource.
type StateShowCommand struct {
	Meta
	StateMeta
}

func (c *StateShowCommand) Run(args []string) int {
	ctx := c.CommandContext()

	args = c.Meta.process(args)
	cmdFlags := c.Meta.defaultFlagSet("state show")
	c.Meta.varFlagSet(cmdFlags)
	cmdFlags.StringVar(&c.Meta.statePath, "state", "", "path")

	showSensitive := false
	cmdFlags.BoolVar(&showSensitive, "show-sensitive", false, "displays sensitive values")

	if err := cmdFlags.Parse(args); err != nil {
		c.Streams.Eprintf("Error parsing command-line flags: %s\n", err.Error())
		return 1
	}
	args = cmdFlags.Args()
	if len(args) != 1 {
		c.Streams.Eprint("Exactly one argument expected.\n")
		return cli.RunResultHelp
	}

	// Check for user-supplied plugin path
	var err error
	if c.pluginPath, err = c.loadPluginPath(); err != nil {
		c.Streams.Eprintf("Error loading plugin path: %\n", err)
		return 1
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

	// We require a local backend
	local, ok := b.(backend.Local)
	if !ok {
		c.Streams.Eprint(ErrUnsupportedLocalOp)
		return 1
	}

	// This is a read-only command
	c.ignoreRemoteVersionConflict(b)

	// Check if the address can be parsed
	addr, addrDiags := addrs.ParseAbsResourceInstanceStr(args[0])
	if addrDiags.HasErrors() {
		c.Streams.Eprintln(fmt.Sprintf(errParsingAddress, args[0]))
		return 1
	}

	// We expect the config dir to always be the cwd
	cwd, err := os.Getwd()
	if err != nil {
		c.Streams.Eprintf("Error getting cwd: %s\n", err)
		return 1
	}

	// Build the operation (required to get the schemas)
	opReq := c.Operation(ctx, b, arguments.ViewHuman, enc)
	opReq.AllowUnsetVariables = true
	opReq.ConfigDir = cwd
	var callDiags tfdiags.Diagnostics
	opReq.RootCall, callDiags = c.rootModuleCall(ctx, opReq.ConfigDir)
	if callDiags.HasErrors() {
		c.showDiagnostics(callDiags)
		return 1
	}

	opReq.ConfigLoader, err = c.initConfigLoader()
	if err != nil {
		c.Streams.Eprintf("Error initializing config loader: %s\n", err)
		return 1
	}

	// Get the context (required to get the schemas)
	lr, _, ctxDiags := local.LocalRun(ctx, opReq)
	if ctxDiags.HasErrors() {
		c.View.Diagnostics(ctxDiags)
		return 1
	}

	// Get the schemas from the context
	schemas, diags := lr.Core.Schemas(lr.Config, lr.InputState)
	if diags.HasErrors() {
		c.View.Diagnostics(diags)
		return 1
	}

	// Get the state
	env, err := c.Workspace(ctx)
	if err != nil {
		c.Streams.Eprintf("Error selecting workspace: %s\n", err)
		return 1
	}
	stateMgr, err := b.StateMgr(env)
	if err != nil {
		c.Streams.Eprintln(fmt.Sprintf(errStateLoadingState, err))
		return 1
	}
	if err := stateMgr.RefreshState(); err != nil {
		c.Streams.Eprintf("Failed to refresh state: %s\n", err)
		return 1
	}

	state := stateMgr.State()
	if state == nil {
		c.Streams.Eprintln(errStateNotFound)
		return 1
	}
	migratedState, migrateDiags := tofumigrate.MigrateStateProviderAddresses(lr.Config, state)
	diags = diags.Append(migrateDiags)
	if migrateDiags.HasErrors() {
		c.View.Diagnostics(diags)
		return 1
	}
	state = migratedState

	is := state.ResourceInstance(addr)
	if !is.HasCurrent() {
		c.Streams.Eprintln(errNoInstanceFound)
		return 1
	}

	// check if the resource has a configured provider, otherwise this will use the default provider
	rs := state.Resource(addr.ContainingResource())
	absPc := addrs.AbsProviderConfig{
		Provider: rs.ProviderConfig.Provider,
		Alias:    rs.ProviderConfig.Alias,
		Module:   addrs.RootModule,
	}
	singleInstance := states.NewState()
	singleInstance.EnsureModule(addr.Module).SetResourceInstanceCurrent(
		addr.Resource,
		is.Current,
		absPc,
		addrs.NoKey,
	)

	root, outputs, err := jsonstate.MarshalForRenderer(statefile.New(singleInstance, "", 0), schemas)
	if err != nil {
		c.Streams.Eprintf("Failed to marshal state to json: %s", err)
	}

	jstate := jsonformat.State{
		StateFormatVersion:    jsonstate.FormatVersion,
		ProviderFormatVersion: jsonprovider.FormatVersion,
		RootModule:            root,
		RootModuleOutputs:     outputs,
		ProviderSchemas:       jsonprovider.MarshalForRenderer(schemas),
	}

	renderer := jsonformat.Renderer{
		Streams:             c.Streams,
		Colorize:            c.Colorize(),
		RunningInAutomation: c.RunningInAutomation,
		ShowSensitive:       showSensitive,
	}

	renderer.RenderHumanState(jstate)
	return 0
}

func (c *StateShowCommand) Help() string {
	helpText := `
Usage: tofu [global options] state show [options] ADDRESS

  Shows the attributes of a resource in the OpenTofu state.

  This command shows the attributes of a single resource in the OpenTofu
  state. The address argument must be used to specify a single resource.
  You can view the list of available resources with "tofu state list".

Options:

  -state=statefile    Path to a OpenTofu state file to use to look
                      up OpenTofu-managed resources. By default it will
                      use the state "terraform.tfstate" if it exists.

  -show-sensitive     If specified, sensitive values will be displayed.

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

func (c *StateShowCommand) Synopsis() string {
	return "Show a resource in the state"
}

const errNoInstanceFound = `No instance found for the given address!

This command requires that the address references one specific instance.
To view the available instances, use "tofu state list". Please modify 
the address to reference a specific instance.`

const errParsingAddress = `Error parsing instance address: %s

This command requires that the address references one specific instance.
To view the available instances, use "tofu state list". Please modify 
the address to reference a specific instance.`
