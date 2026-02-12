// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/repl"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

// ConsoleCommand is a Command implementation that starts an interactive
// console that can be used to try expressions with the current config.
type ConsoleCommand struct {
	Meta
}

func (c *ConsoleCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()

	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)
	// Because the legacy UI was using println to show diagnostics and the new view is using, by default, print,
	// in order to keep functional parity, we setup the view to add a new line after each diagnostic.
	c.View.DiagsWithNewline()

	// Propagate -no-color for legacy use of Ui. The remote backend and
	// cloud package use this; it should be removed when/if they are
	// migrated to views.
	c.Meta.color = !common.NoColor
	c.Meta.Color = c.Meta.color

	// Parse and validate flags
	args, closer, diags := arguments.ParseConsole(rawArgs)
	defer closer()

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewConsole(args.ViewOptions, c.View)
	// ... and initialise the Meta.Ui to wrap Meta.View into a new implementation
	// that is able to print by using View abstraction and use the Meta.Ui
	// to ask for the user input.
	c.Meta.configureUiFromView(args.ViewOptions)
	if diags.HasErrors() {
		view.HelpPrompt()
		view.Diagnostics(diags)
		return 1
	}
	// TODO meta-refactor: get rid of this assignment once the statePath from Meta is removed
	c.Meta.statePath = args.StatePath
	c.Meta.stateLock = args.Backend.StateLock
	c.Meta.stateLockTimeout = args.Backend.StateLockTimeout

	// FIXME: the -input flag value is needed to initialize the backend and the
	// operation, but there is no clear path to pass this value down, so we
	// continue to mutate the Meta object state for now.
	c.Meta.input = args.ViewOptions.InputEnabled
	c.GatherVariables(args.Vars)

	configPath := c.WorkingDir.NormalizePath(c.WorkingDir.RootModuleDir())

	// Check for user-supplied plugin path
	var err error
	if c.pluginPath, err = c.loadPluginPath(); err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Plugins loading error",
			fmt.Sprintf("Error loading plugin path: %s", err),
		)))
		return 1
	}

	// Load the encryption configuration
	enc, encDiags := c.EncryptionFromPath(ctx, configPath)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	backendConfig, backendDiags := c.loadBackendConfig(ctx, configPath)
	diags = diags.Append(backendDiags)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Load the backend
	b, backendDiags := c.Backend(ctx, &BackendOpts{
		Config: backendConfig,
	}, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// We require a local backend
	local, ok := b.(backend.Local)
	if !ok {
		view.Diagnostics(diags) // in case of any warnings in here
		view.UnsupportedLocalOp()
		return 1
	}

	// This is a read-only command
	c.ignoreRemoteVersionConflict(b)

	// Build the operation
	opReq := c.Operation(ctx, b, arguments.ViewOptions{ViewType: arguments.ViewHuman}, enc)
	opReq.ConfigDir = configPath
	opReq.ConfigLoader, err = c.initConfigLoader()
	opReq.AllowUnsetVariables = true // we'll just evaluate them as unknown
	if err != nil {
		diags = diags.Append(err)
		view.Diagnostics(diags)
		return 1
	}

	{
		// Setup required variables/call for operation (usually done in Meta.RunOperation)
		var moreDiags, callDiags tfdiags.Diagnostics
		opReq.Variables, moreDiags = c.collectVariableValues()
		opReq.RootCall, callDiags = c.rootModuleCall(ctx, opReq.ConfigDir)
		diags = diags.Append(moreDiags).Append(callDiags)
		if moreDiags.HasErrors() {
			view.Diagnostics(diags)
			return 1
		}
	}

	// Get the context
	lr, _, ctxDiags := local.LocalRun(ctx, opReq)
	diags = diags.Append(ctxDiags)
	if ctxDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Successfully creating the context can result in a lock, so ensure we release it
	defer func() {
		diags := opReq.StateLocker.Unlock()
		if diags.HasErrors() {
			view.Diagnostics(diags)
		}
	}()

	evalOpts := &tofu.EvalOpts{}
	if lr.PlanOpts != nil {
		// the LocalRun type is built primarily to support the main operations,
		// so the variable values end up in the "PlanOpts" even though we're
		// not actually making a plan.
		evalOpts.SetVariables = lr.PlanOpts.SetVariables
	}

	// Before we can evaluate expressions, we must compute and populate any
	// derived values (input variables, local values, output values)
	// that are not stored in the persistent state.
	scope, scopeDiags := lr.Core.Eval(ctx, lr.Config, lr.InputState, addrs.RootModuleInstance, evalOpts)
	diags = diags.Append(scopeDiags)
	if scope == nil {
		// scope is nil if there are errors so bad that we can't even build a scope.
		// Otherwise, we'll try to eval anyway.
		view.Diagnostics(diags)
		return 1
	}

	// set the ConsoleMode to true so any available console-only functions included.
	scope.ConsoleMode = true

	if diags.HasErrors() {
		diags = diags.Append(tfdiags.SimpleWarning("Due to the problems above, some expressions may produce unexpected results."))
	}

	// Before we become interactive we'll show any diagnostics we encountered
	// during initialization, and then afterwards the driver will manage any
	// further diagnostics itself.
	view.Diagnostics(diags)

	// IO Loop
	session := &repl.Session{
		Scope: scope,
	}

	// Determine if stdin is a pipe. If so, we evaluate directly.
	if c.StdinPiped() {
		return c.modePiped(session, view)
	}

	return c.modeInteractive(session, view)
}

func (c *ConsoleCommand) modePiped(session *repl.Session, view views.Console) int {
	scanner := bufio.NewScanner(os.Stdin)

	var consoleState consoleBracketState

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// we check if there is no escaped new line at the end, or any open brackets
		// if we have neither, then we can execute
		fullCommand, bracketState := consoleState.UpdateState(line)
		if bracketState <= 0 {
			result, exit, diags := session.Handle(fullCommand)
			if diags.HasErrors() {
				// We're in piped mode, so we'll exit immediately on error.
				view.Diagnostics(diags)
				return 1
			}
			if exit {
				return 0
			}
			// Output the result
			view.Output(result)
		}
	}

	return 0
}

func (c *ConsoleCommand) Help() string {
	helpText := `
Usage: tofu [global options] console [options]

  Starts an interactive console for experimenting with OpenTofu
  interpolations.

  This will open an interactive console that you can use to type
  interpolations into and inspect their values. This command loads the
  current state. This lets you explore and test interpolations before
  using them in future configurations.

  This command will never modify your state.

Options:

  -compact-warnings      If OpenTofu produces any warnings that are not
                         accompanied by errors, show them in a more compact
                         form that includes only the summary messages.

  -consolidate-warnings  If OpenTofu produces any warnings, no consolidation
                         will be performed. All locations, for all warnings
                         will be listed. Enabled by default.

  -consolidate-errors    If OpenTofu produces any errors, no consolidation
                         will be performed. All locations, for all errors
                         will be listed. Disabled by default

  -state=path            Legacy option for the local backend only. See the local
                         backend's documentation for more information.

  -var 'foo=bar'         Set a variable in the OpenTofu configuration. This
                         flag can be set multiple times.

  -var-file=foo          Set variables in the OpenTofu configuration from
                         a file. If "terraform.tfvars" or any ".auto.tfvars"
                         files are present, they will be automatically loaded.

  -lock=false            Don't hold a state lock during the operation. This is
                         dangerous if others might concurrently run commands
                         against the same workspace.

  -lock-timeout=0s       Duration to retry a state lock.
`
	return strings.TrimSpace(helpText)
}

func (c *ConsoleCommand) Synopsis() string {
	return "Try OpenTofu expressions at an interactive command prompt"
}

// TODO meta-refactor: move this to arguments once all commands are using the same shim logic
func (c *ConsoleCommand) GatherVariables(args *arguments.Vars) {
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
