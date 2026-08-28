// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"fmt"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func OutputCommander() Command {
	cmd := Command{
		Name:  "output",
		Short: "Show output values from your root module",
		Long:  `Reads an output variable from a OpenTofu state file and prints the value. With no additional arguments, output will display all the outputs for the root module.  If NAME is not specified, all outputs are printed.`,
	}

	args := arguments.BindOutput(&cmd.CommandLine)
	cmd.Run = func(meta Meta) int {
		return OutputCommand{meta}.Execute(args, views.NewOutput(args.View, meta.View))
	}

	return cmd
}

// OutputCommand is a Command implementation that reads an output
// from a OpenTofu state and prints it.
type OutputCommand struct {
	Meta
}

func (c OutputCommand) Execute(args *arguments.Output, view views.Output) int {
	ctx := c.CommandContext()

	// Load the encryption configuration
	enc, diags := c.Encryption(ctx)
	if diags.HasErrors() {
		c.View.Diagnostics(diags)
		return 1
	}

	// Fetch data from state
	outputs, diags := c.Outputs(ctx, args.State.StatePath, enc)
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
