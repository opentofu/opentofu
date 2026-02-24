// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tracing"
)

// GetCommand is a Command implementation that takes a OpenTofu
// configuration and downloads all the modules.
type GetCommand struct {
	Meta
}

func (c *GetCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()
	ctx, span := tracing.Tracer().Start(ctx, "Get")
	defer span.End()

	// new view
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
	args, closer, diags := arguments.ParseGet(rawArgs)
	defer closer()

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewGet(args.ViewOptions, c.View)
	// ... and initialise the Meta.Ui to wrap Meta.View into a new implementation
	// that is able to print by using View abstraction and use the Meta.Ui
	// to ask for the user input.
	c.Meta.configureUiFromView(args.ViewOptions)

	if diags.HasErrors() {
		view.Diagnostics(diags)
		return cli.RunResultHelp
	}
	c.GatherVariables(args.Vars)

	// Initialization can be aborted by interruption signals
	ctx, done := c.InterruptibleContext(ctx)
	defer done()

	// This gets the current directory as full path.
	path := c.WorkingDir.NormalizePath(c.WorkingDir.RootModuleDir())

	abort, diags := getModules(ctx, &c.Meta, path, args.TestsDirectory, args.Update, view)
	view.Diagnostics(diags)
	if abort || diags.HasErrors() {
		return 1
	}

	return 0
}

// TODO meta-refactor: move this to arguments once all commands are using the same shim logic
func (c *GetCommand) GatherVariables(args *arguments.Vars) {
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

func (c *GetCommand) Help() string {
	helpText := `
Usage: tofu [global options] get [options]

  Downloads and installs modules needed for the configuration in the 
  current working directory.

  This recursively downloads all modules needed, such as modules
  imported by modules imported by the root and so on. If a module is
  already downloaded, it will not be redownloaded or checked for updates
  unless the -update flag is specified.

  Module installation also happens automatically by default as part of
  the "tofu init" command, so you should rarely need to run this
  command separately.

Options:

  -update               Check already-downloaded modules for available updates
                        and install the newest versions available.

  -no-color             Disable text coloring in the output.

  -test-directory=path  Set the OpenTofu test directory, defaults to "tests". When set, the
                        test command will search for test files in the current directory and
                        in the one specified by the flag.

  -json                 Produce output in a machine-readable JSON format, 
                        suitable for use in text editor integrations and other 
                        automated systems. Always disables color.

  -json-into=out.json   Produce the same output as -json, but sent directly
                        to the given file. This allows automation to preserve
                        the original human-readable output streams, while
                        capturing more detailed logs for machine analysis.

  -var 'foo=bar'        Set a value for one of the input variables in the root
                        module of the configuration. Use this option more than
                        once to set more than one variable.

  -var-file=filename    Load variable values from the given file, in addition
                        to the default files terraform.tfvars and *.auto.tfvars.
                        Use this option more than once to include more than one
                        variables file.

`
	return strings.TrimSpace(helpText)
}

func (c *GetCommand) Synopsis() string {
	return "Install or upgrade remote OpenTofu modules"
}

func getModules(ctx context.Context, m *Meta, path string, testsDir string, upgrade bool, view views.Get) (abort bool, diags tfdiags.Diagnostics) {
	hooks := view.Hooks(true)
	return m.installModules(ctx, path, testsDir, upgrade, true, hooks)
}
