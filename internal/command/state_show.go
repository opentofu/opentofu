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
	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/command/views"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/arguments"
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

func (c *StateShowCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()

	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)
	// Because the legacy UI was using println to show diagnostics and the new view is using, by default, print,
	// in order to keep functional parity, we setup the view to add a new line after each diagnostic.
	c.View.DiagsWithNewline()

	// Propagate -no-color for legacy use of Ui. The remote backend and
	// cloud package use this; it should be removed when/if they are
	// migrated to views.
	// We need this down the road for the confirmation
	c.Meta.color = !common.NoColor
	c.Meta.Color = c.Meta.color

	// Parse and validate flags
	args, closer, diags := arguments.ParseStateShow(rawArgs)
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
	c.View.SetShowSensitive(args.ShowSensitive)
	// TODO meta-refactor: remove these assignments once we have a clear way to propagate these to the logic
	//  that uses them
	c.GatherVariables(args.Vars)
	c.statePath = args.StatePath

	// Check for user-supplied plugin path
	var err error
	if c.pluginPath, err = c.loadPluginPath(); err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Error loading plugin path: %s", err)))
		return 1
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

	// We require a local backend
	local, ok := b.(backend.Local)
	if !ok {
		view.UnsupportedLocalOp()
		return 1
	}

	// This is a read-only command
	c.ignoreRemoteVersionConflict(b)

	// Check if the address can be parsed
	addr, addrDiags := addrs.ParseAbsResourceInstanceStr(args.TargetRawAddr)
	if addrDiags.HasErrors() {
		view.AddressParsingError(args.TargetRawAddr)
		return 1
	}

	// We expect the config dir to always be the cwd
	cwd := c.WorkingDir.NormalizePath(c.WorkingDir.RootModuleDir())

	// Build the operation (required to get the schemas)
	opReq := c.Operation(ctx, b, args.ViewOptions, enc)
	opReq.AllowUnsetVariables = true
	opReq.ConfigDir = cwd
	var callDiags tfdiags.Diagnostics
	opReq.RootCall, callDiags = c.rootModuleCall(ctx, opReq.ConfigDir)
	if callDiags.HasErrors() {
		view.Diagnostics(callDiags)
		return 1
	}

	opReq.ConfigLoader, err = c.initConfigLoader()
	if err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Error initializing config loader: %s", err)))
		return 1
	}

	// Get the context (required to get the schemas)
	lr, _, ctxDiags := local.LocalRun(ctx, opReq)
	if ctxDiags.HasErrors() {
		view.Diagnostics(ctxDiags)
		return 1
	}

	// Get the schemas from the context
	schemas, diags := lr.Core.Schemas(ctx, lr.Config, lr.InputState)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Get the state
	env, err := c.Workspace(ctx)
	if err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Error selecting workspace: %s\n", err)))
		return 1
	}
	stateMgr, err := b.StateMgr(ctx, env)
	if err != nil {
		view.StateLoadingFailure(err.Error())
		return 1
	}
	if err := stateMgr.RefreshState(context.TODO()); err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Failed to refresh state: %s", err)))
		return 1
	}

	state := stateMgr.State()
	if state == nil {
		view.StateNotFound()
		return 1
	}
	migratedState, migrateDiags := tofumigrate.MigrateStateProviderAddresses(lr.Config, state)
	diags = diags.Append(migrateDiags)
	if migrateDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}
	state = migratedState

	is := state.ResourceInstance(addr)
	if !is.HasCurrent() {
		view.NoInstanceFoundError()
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
	resourceState := statefile.New(singleInstance, "", 0)
	return view.ShowResourceState(ctx, resourceState, schemas)
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

  -json               Produce output in a machine-readable JSON format, 
                      suitable for use in text editor integrations and other 
                      automated systems. Always disables color.
                      Warning: Using this option will always print the 
                      sensitive values even if '-show-sensitive' is not 
                      specified.

  -json-into=out.json Produce the same output as -json, but sent directly
                      to the given file. This allows automation to preserve
                      the original human-readable output streams, while
                      capturing more detailed logs for machine analysis.
                      Warning: Using this option will always print the 
                      sensitive values even if '-show-sensitive' is not 
                      specified.

`
	return strings.TrimSpace(helpText)
}

func (c *StateShowCommand) Synopsis() string {
	return "Show a resource in the state"
}

// TODO meta-refactor: move this to arguments once all commands are using the same shim logic
func (c *StateShowCommand) GatherVariables(args *arguments.Vars) {
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
