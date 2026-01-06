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
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// OutputCommand is a Command implementation that reads an output
// from a OpenTofu state and prints it.
type OutputCommand struct {
	Meta
}

func (c *OutputCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()
	// Parse and apply global view arguments
	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)

	// Parse and validate flags
	args, closer, diags := arguments.ParseOutput(rawArgs)
	defer closer()
	if diags.HasErrors() {
		c.View.Diagnostics(diags)
		c.View.HelpPrompt("output")
		return 1
	}

	c.View.SetShowSensitive(args.ShowSensitive)

	view := views.NewOutput(args.ViewOptions, c.View)

	// Inject variables from args into meta for static evaluation
	c.GatherVariables(args.Vars)

	// Load the encryption configuration
	enc, encDiags := c.Encryption(ctx)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		c.View.Diagnostics(diags)
		return 1
	}

	// Fetch data from state
	outputs, diags := c.Outputs(ctx, args.StatePath, enc)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Render the view
	viewDiags := view.Output(args.Name, outputs)
	diags = diags.Append(viewDiags)

	view.Diagnostics(diags)

	if diags.HasErrors() {
		return 1
	}

	return 0
}

func (c *OutputCommand) Outputs(ctx context.Context, statePath string, enc encryption.Encryption) (map[string]*states.OutputValue, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Allow state path override
	if statePath != "" {
		c.Meta.statePath = statePath
	}

	// Load the backend
	b, backendDiags := c.Backend(ctx, nil, enc.State())
	diags = diags.Append(backendDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	// This is a read-only command
	c.ignoreRemoteVersionConflict(b)

	env, err := c.Workspace(ctx)
	if err != nil {
		diags = diags.Append(fmt.Errorf("Error selecting workspace: %w", err))
		return nil, diags
	}

	// Get the state
	stateStore, err := b.StateMgr(ctx, env)
	if err != nil {
		diags = diags.Append(fmt.Errorf("Failed to load state: %w", err))
		return nil, diags
	}

	output, err := stateStore.GetRootOutputValues(context.TODO())
	if err != nil {
		return nil, diags.Append(err)
	}

	return output, diags
}

func (c *OutputCommand) GatherVariables(args *arguments.Vars) {
	// FIXME the arguments package currently trivially gathers variable related
	// arguments in a heterogeneous slice, in order to minimize the number of
	// code paths gathering variables during the transition to this structure.
	// Once all commands that gather variables have been converted to this
	// structure, we could move the variable gathering code to the arguments
	// package directly, removing this shim layer.

	varArgs := args.All()
	items := make([]rawFlag, len(varArgs))
	for i := range varArgs {
		items[i].Name = varArgs[i].Name
		items[i].Value = varArgs[i].Value
	}
	c.Meta.variableArgs = rawFlags{items: &items}
}

func (c *OutputCommand) Help() string {
	helpText := `
Usage: tofu [global options] output [options] [NAME]

  Reads an output variable from a OpenTofu state file and prints
  the value. With no additional arguments, output will display all
  the outputs for the root module.  If NAME is not specified, all
  outputs are printed.

Options:

  -state=path          Path to the state file to read. Defaults to
                       "terraform.tfstate". Ignored when remote 
                       state is used.
                      
  -no-color            If specified, output won't contain any color.
                      
  -json                If specified, machine readable output will be
                       printed in JSON format.

  -json-into=out.json  Produce the same output as -json, but sent directly
                       to the given file. This allows automation to preserve
                       the original human-readable output streams, while
                       capturing more detailed logs for machine analysis.

  -raw                 For value types that can be automatically
                       converted to a string, will print the raw
                       string directly, rather than a human-oriented
                       representation of the value.
                       
                       Use this with care when stdout is a terminal and when
                       the output value might contain control characters.
                       
  -show-sensitive      If specified, sensitive values will be displayed.
                       
  -var 'foo=bar'       Set a value for one of the input variables in the root
                       module of the configuration. Use this option more than
                       once to set more than one variable.
                       
  -var-file=filename   Load variable values from the given file, in addition
                       to the default files terraform.tfvars and *.auto.tfvars.
                       Use this option more than once to include more than one
                       variables file.
`
	return strings.TrimSpace(helpText)
}

func (c *OutputCommand) Synopsis() string {
	return "Show output values from your root module"
}
