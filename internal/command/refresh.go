// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"fmt"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func RefreshCommander() Command {
	cmd := Command{
		Name:  "refresh",
		Short: "Update the state to match remote systems",
		Long: `Update the state file of your infrastructure with metadata that matches the physical resources they are tracking.

This will not modify your infrastructure, but it can modify your state file to update metadata. This metadata might cause new changes to occur when you generate a plan or call apply next.`,
	}

	args := arguments.BindRefresh(&cmd.CommandLine)
	cmd.Run = func(meta Meta) int {
		return RefreshCommand{meta}.Execute(args, views.NewRefresh(args.View, meta.View))
	}

	return cmd
}

// RefreshCommand is a cli.Command implementation that refreshes the state
// file.
type RefreshCommand struct {
	Meta
}

func (c RefreshCommand) Execute(args *arguments.Refresh, view views.Refresh) int {
	var diags tfdiags.Diagnostics
	ctx := c.CommandContext()
	ctx = tfdiags.ContextWithLintFilterHints(ctx, args.View.LintInclude, args.View.LintExclude)
	diags = diags.Append(tfdiags.ExperimentalLintWarn(ctx))

	// Check for user-supplied plugin path
	var err error
	if c.pluginPath, err = c.loadPluginPath(); err != nil {
		diags = diags.Append(err)
		view.Diagnostics(diags)
		return 1
	}

	// Load the encryption configuration
	enc, encDiags := c.Encryption(ctx)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Prepare the backend with the backend-specific arguments
	be, beDiags := c.PrepareBackend(ctx, args.State, view, enc)
	diags = diags.Append(beDiags)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Build the operation request
	opReq, opDiags := c.OperationRequest(ctx, be, view, args.Operation, enc)
	diags = diags.Append(opDiags)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Before we delegate to the backend, we'll print any warning diagnostics
	// we've accumulated here, since the backend will start fresh with its own
	// diagnostics.
	view.Diagnostics(diags)
	diags = nil

	// Perform the operation
	op, diags := c.RunOperation(ctx, be, opReq)
	view.Diagnostics(diags)
	if diags.HasErrors() {
		return 1
	}

	if op.State != nil {
		view.Outputs(op.State.RootModule().OutputValues)
	}

	return op.Result.ExitStatus()
}

func (c *RefreshCommand) PrepareBackend(ctx context.Context, args *arguments.State, view views.Refresh, enc encryption.Encryption) (backend.Enhanced, tfdiags.Diagnostics) {
	backendConfig, diags := c.loadBackendConfig(ctx, ".")
	if diags.HasErrors() {
		return nil, diags
	}

	// Load the backend
	be, beDiags := c.Backend(ctx, &BackendOpts{
		Config: backendConfig,
		View:   view.Backend(),
	}, enc.State())
	diags = diags.Append(beDiags)
	if beDiags.HasErrors() {
		return nil, diags
	}

	return be, diags
}

func (c *RefreshCommand) OperationRequest(ctx context.Context, be backend.Enhanced, view views.Refresh, args *arguments.Operation, enc encryption.Encryption,
) (*backend.Operation, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Build the operation
	opReq := c.Operation(ctx, be, view.Backend(), enc)
	opReq.ConfigDir = "."
	opReq.Hooks = view.Hooks()
	opReq.Targets = args.Targets
	opReq.Excludes = args.Excludes
	opReq.Type = backend.OperationTypeRefresh
	opReq.View = view.Operation()

	var err error
	opReq.ConfigLoader, err = configload.Initialise(c.configLoader())
	if err != nil {
		diags = diags.Append(fmt.Errorf("Failed to initialize config loader: %w", err))
		return nil, diags
	}

	return opReq, diags
}
