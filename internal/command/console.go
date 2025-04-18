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
	"github.com/opentofu/opentofu/internal/repl"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"

	"github.com/mitchellh/cli"
)

// ConsoleCommand is a Command implementation that starts an interactive
// console that can be used to try expressions with the current config.
type ConsoleCommand struct {
	Meta
}

func (c *ConsoleCommand) Run(args []string) int {
	ctx := c.CommandContext()

	args = c.Meta.process(args)
	cmdFlags := c.Meta.extendedFlagSet("console")
	cmdFlags.StringVar(&c.Meta.statePath, "state", DefaultStateFilename, "path")
	cmdFlags.Usage = func() { c.Ui.Error(c.Help()) }
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command line flags: %s\n", err.Error()))
		return 1
	}

	configPath, err := modulePath(cmdFlags.Args())
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}
	configPath = c.Meta.normalizePath(configPath)

	// Check for user-supplied plugin path
	if c.pluginPath, err = c.loadPluginPath(); err != nil {
		c.Ui.Error(fmt.Sprintf("Error loading plugin path: %s", err))
		return 1
	}

	var diags tfdiags.Diagnostics

	// Load the encryption configuration
	enc, encDiags := c.EncryptionFromPath(configPath)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	backendConfig, backendDiags := c.loadBackendConfig(configPath)
	diags = diags.Append(backendDiags)
	if diags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	// Load the backend
	b, backendDiags := c.Backend(&BackendOpts{
		Config: backendConfig,
	}, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	// We require a local backend
	local, ok := b.(backend.Local)
	if !ok {
		c.showDiagnostics(diags) // in case of any warnings in here
		c.Ui.Error(ErrUnsupportedLocalOp)
		return 1
	}

	// This is a read-only command
	c.ignoreRemoteVersionConflict(b)

	// Build the operation
	opReq := c.Operation(b, arguments.ViewHuman, enc)
	opReq.ConfigDir = configPath
	opReq.ConfigLoader, err = c.initConfigLoader()
	opReq.AllowUnsetVariables = true // we'll just evaluate them as unknown
	if err != nil {
		diags = diags.Append(err)
		c.showDiagnostics(diags)
		return 1
	}

	{
		// Setup required variables/call for operation (usually done in Meta.RunOperation)
		var moreDiags, callDiags tfdiags.Diagnostics
		opReq.Variables, moreDiags = c.collectVariableValues()
		opReq.RootCall, callDiags = c.rootModuleCall(opReq.ConfigDir)
		diags = diags.Append(moreDiags).Append(callDiags)
		if moreDiags.HasErrors() {
			c.showDiagnostics(diags)
			return 1
		}
	}

	// Get the context
	lr, _, ctxDiags := local.LocalRun(ctx, opReq)
	diags = diags.Append(ctxDiags)
	if ctxDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	// Successfully creating the context can result in a lock, so ensure we release it
	defer func() {
		diags := opReq.StateLocker.Unlock()
		if diags.HasErrors() {
			c.showDiagnostics(diags)
		}
	}()

	// Set up the UI so we can output directly to stdout
	ui := &cli.BasicUi{
		Writer:      os.Stdout,
		ErrorWriter: os.Stderr,
	}

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
		c.showDiagnostics(diags)
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
	c.showDiagnostics(diags)

	// IO Loop
	session := &repl.Session{
		Scope: scope,
	}

	// Determine if stdin is a pipe. If so, we evaluate directly.
	if c.StdinPiped() {
		return c.modePiped(session, ui)
	}

	return c.modeInteractive(session, ui)
}

func (c *ConsoleCommand) modePiped(session *repl.Session, ui cli.Ui) int {
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
				c.showDiagnostics(diags)
				return 1
			}
			if exit {
				return 0
			}
			// Output the result
			ui.Output(result)
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
`
	return strings.TrimSpace(helpText)
}

func (c *ConsoleCommand) Synopsis() string {
	return "Try OpenTofu expressions at an interactive command prompt"
}
