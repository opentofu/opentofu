// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// GetCommand is a Command implementation that takes a OpenTofu
// configuration and downloads all the modules.
type GetCommand struct {
	Meta
}

func (c *GetCommand) Run(args []string) int {
	var update bool
	var testsDirectory string

	args = c.Meta.process(args)
	cmdFlags := c.Meta.defaultFlagSet("get")
	c.Meta.varFlagSet(cmdFlags)
	cmdFlags.BoolVar(&update, "update", false, "update")
	cmdFlags.StringVar(&testsDirectory, "test-directory", "tests", "test-directory")
	cmdFlags.BoolVar(&c.outputInJSON, "json", false, "json")
	cmdFlags.StringVar(&c.outputJSONInto, "json-into", "", "json-into")
	cmdFlags.Usage = func() { c.Ui.Error(c.Help()) }
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line flags: %s\n", err.Error()))
		return 1
	}
	if c.outputInJSON {
		c.Meta.color = false
		c.Meta.Color = false
		c.oldUi = c.Ui
		c.Ui = &WrappedUi{
			cliUi:            c.oldUi,
			jsonView:         views.NewJSONView(c.View, nil),
			onlyOutputInJSON: true,
		}
	}

	if c.outputJSONInto != "" {
		if c.outputInJSON {
			// Not a valid combination
			c.Ui.Error("The -json and -json-into options are mutually-exclusive in their use")
			return 1
		}

		out, closer, diags := arguments.OpenJSONIntoFile(c.outputJSONInto)
		defer closer()
		if diags.HasErrors() {
			c.Ui.Error(diags.Err().Error())
			return 1
		}

		c.oldUi = c.Ui
		c.Ui = &WrappedUi{
			cliUi:            c.oldUi,
			jsonView:         views.NewJSONView(c.View, out),
			onlyOutputInJSON: false,
		}
	}

	// Initialization can be aborted by interruption signals
	ctx, done := c.InterruptibleContext(c.CommandContext())
	defer done()

	path, err := modulePath(cmdFlags.Args())
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	path = c.Meta.WorkingDir.NormalizePath(path)

	abort, diags := getModules(ctx, &c.Meta, path, testsDirectory, update)
	c.showDiagnostics(diags)
	if abort || diags.HasErrors() {
		return 1
	}

	return 0
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

func getModules(ctx context.Context, m *Meta, path string, testsDir string, upgrade bool) (abort bool, diags tfdiags.Diagnostics) {
	hooks := uiModuleInstallHooks{
		Ui:             m.Ui,
		ShowLocalPaths: true,
	}
	return m.installModules(ctx, path, testsDir, upgrade, true, hooks)
}
