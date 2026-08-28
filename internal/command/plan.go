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

func PlanCommander() Command {
	cmd := Command{
		Name:  "plan",
		Short: "Show changes required by the current configuration",
		Long: `Generates a speculative execution plan, showing what actions OpenTofu would take to apply the current configuration. This command will not actually perform the planned actions.

You can optionally save the plan to a file, which you can then pass to the "apply" command to perform exactly the actions described in the plan.`,

		GroupID: MainCommandGroup.ID,
	}

	args := arguments.BindPlan(&cmd.CommandLine)
	cmd.Run = func(meta Meta) int {
		return PlanCommand{meta}.Execute(args, views.NewPlan(args.View, meta.View))
	}

	return cmd
}

// PlanCommand is a Command implementation that compares a OpenTofu
// configuration to an actual infrastructure and shows the differences.
type PlanCommand struct {
	Meta
}

func (c PlanCommand) Execute(args *arguments.Plan, view views.Plan) int {
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

	diags = diags.Append(c.providerDevOverrideRuntimeWarnings())

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
	opReq, opDiags := c.OperationRequest(ctx, be, view, args.View, args.Operation, args.OutPath, args.GenerateConfigPath, enc)
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

	if op.Result != backend.OperationSuccess {
		return op.Result.ExitStatus()
	}
	if args.DetailedExitCode && !op.PlanEmpty {
		return 2
	}

	return op.Result.ExitStatus()
}

func (c *PlanCommand) PrepareBackend(ctx context.Context, args *arguments.State, view views.Plan, enc encryption.Encryption) (backend.Enhanced, tfdiags.Diagnostics) {
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

func (c *PlanCommand) OperationRequest(
	ctx context.Context,
	be backend.Enhanced,
	view views.Plan,
	viewOptions *arguments.View,
	args *arguments.Operation,
	planOutPath string,
	generateConfigOut string,
	enc encryption.Encryption,
) (*backend.Operation, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Build the operation
	opReq := c.Operation(ctx, be, view.Backend(), enc)
	opReq.ConfigDir = "."
	opReq.PlanMode = args.PlanMode
	opReq.Hooks = view.Hooks()
	opReq.PlanRefresh = args.Refresh
	opReq.PlanOutPath = planOutPath
	opReq.GenerateConfigOut = generateConfigOut
	opReq.Targets = args.Targets
	opReq.Excludes = args.Excludes
	opReq.ForceReplace = args.ForceReplace
	opReq.Type = backend.OperationTypePlan
	opReq.View = view.Operation()

	var err error
	opReq.ConfigLoader, err = configload.Initialise(c.configLoader())
	if err != nil {
		diags = diags.Append(fmt.Errorf("Failed to initialize config loader: %w", err))
		return nil, diags
	}

	return opReq, diags
}
