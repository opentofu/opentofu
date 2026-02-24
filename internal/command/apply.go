// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/plans/planfile"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ApplyCommand is a Command implementation that applies a OpenTofu
// configuration and actually builds or changes infrastructure.
type ApplyCommand struct {
	Meta

	// If true, then this apply command will become the "destroy"
	// command. It is just like apply but only processes a destroy.
	Destroy bool
}

func (c *ApplyCommand) Run(rawArgs []string) int {
	var diags tfdiags.Diagnostics
	ctx := c.CommandContext()

	// Parse and apply global view arguments
	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)

	// Propagate -no-color for legacy use of Ui.  The remote backend and
	// cloud package use this; it should be removed when/if they are
	// migrated to views.
	c.Meta.color = !common.NoColor
	c.Meta.Color = c.Meta.color

	// Parse and validate flags
	var args *arguments.Apply
	var closer func()
	switch {
	case c.Destroy:
		args, closer, diags = arguments.ParseApplyDestroy(rawArgs)
	default:
		args, closer, diags = arguments.ParseApply(rawArgs)
	}
	defer closer()

	c.View.SetShowSensitive(args.ShowSensitive)

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewApply(args.ViewOptions, c.Destroy, c.View)

	if diags.HasErrors() {
		view.Diagnostics(diags)
		view.HelpPrompt()
		return 1
	}

	// Check for user-supplied plugin path
	var err error
	if c.pluginPath, err = c.loadPluginPath(); err != nil {
		diags = diags.Append(err)
		view.Diagnostics(diags)
		return 1
	}

	// Inject variables from args into meta for static evaluation
	c.GatherVariables(args.Vars)

	// Load the encryption configuration
	enc, encDiags := c.Encryption(ctx)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Attempt to load the plan file, if specified
	planFile, diags := c.LoadPlanFile(args.PlanPath, enc)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// FIXME: the -input flag value is needed to initialize the backend and the
	// operation, but there is no clear path to pass this value down, so we
	// continue to mutate the Meta object state for now.
	c.Meta.input = args.ViewOptions.InputEnabled

	// FIXME: the -parallelism flag is used to control the concurrency of
	// OpenTofu operations. At the moment, this value is used both to
	// initialize the backend via the ContextOpts field inside CLIOpts, and to
	// set a largely unused field on the Operation request. Again, there is no
	// clear path to pass this value down, so we continue to mutate the Meta
	// object state for now.
	c.Meta.parallelism = args.Operation.Parallelism

	// Prepare the backend, passing the plan file if present, and the
	// backend-specific arguments
	be, beDiags := c.PrepareBackend(ctx, planFile, args.State, args.ViewOptions, enc.State())
	diags = diags.Append(beDiags)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Build the operation request
	opReq, opDiags := c.OperationRequest(ctx, be, view, args, planFile, enc)
	diags = diags.Append(opDiags)

	// Before we delegate to the backend, we'll print any warning diagnostics
	// we've accumulated here, since the backend will start fresh with its own
	// diagnostics.
	view.Diagnostics(diags)
	if diags.HasErrors() {
		return 1
	}
	diags = nil

	// Run the operation
	op, diags := c.RunOperation(ctx, be, opReq)
	view.Diagnostics(diags)
	if diags.HasErrors() {
		return 1
	}

	if op.Result != backend.OperationSuccess {
		return op.Result.ExitStatus()
	}

	// Render the resource count and outputs, unless those counts are being
	// rendered already in a remote OpenTofu process.
	if rb, isRemoteBackend := be.(BackendWithRemoteTerraformVersion); !isRemoteBackend || rb.IsLocalOperations() {
		view.ResourceCount(args.State.StateOutPath)
		if !c.Destroy && op.State != nil {
			view.Outputs(op.State.RootModule().OutputValues)
		}
	}

	view.Diagnostics(diags)

	if diags.HasErrors() {
		return 1
	}

	return 0
}

func (c *ApplyCommand) LoadPlanFile(path string, enc encryption.Encryption) (*planfile.WrappedPlanFile, tfdiags.Diagnostics) {
	var planFile *planfile.WrappedPlanFile
	var diags tfdiags.Diagnostics

	// Try to load plan if path is specified
	if path != "" {
		var err error
		planFile, err = c.PlanFile(path, enc.Plan())
		if err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				fmt.Sprintf("Failed to load %q as a plan file", path),
				fmt.Sprintf("Error: %s", err),
			))
			return nil, diags
		}

		// If the path doesn't look like a plan, both planFile and err will be
		// nil. In that case, the user is probably trying to use the positional
		// argument to specify a configuration path. Point them at -chdir.
		if planFile == nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				fmt.Sprintf("Failed to load %q as a plan file", path),
				"The specified path is a directory, not a plan file. You can use the global -chdir flag to use this directory as the configuration root.",
			))
			return nil, diags
		}

		// If we successfully loaded a plan but this is a destroy operation,
		// explain that this is not supported.
		if c.Destroy {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Destroy can't be called with a plan file",
				fmt.Sprintf("If this plan was created using plan -destroy, apply it using:\n  tofu apply %q", path),
			))
			return nil, diags
		}
	}

	return planFile, diags
}

func (c *ApplyCommand) PrepareBackend(ctx context.Context, planFile *planfile.WrappedPlanFile, args *arguments.State, viewOptions arguments.ViewOptions, enc encryption.StateEncryption) (backend.Enhanced, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// FIXME: we need to apply the state arguments to the meta object here
	// because they are later used when initializing the backend. Carving a
	// path to pass these arguments to the functions that need them is
	// difficult but would make their use easier to understand.
	c.Meta.applyStateArguments(args)

	// Load the backend
	var be backend.Enhanced
	var beDiags tfdiags.Diagnostics
	if lp, ok := planFile.Local(); ok {
		plan, err := lp.ReadPlan()
		if err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Failed to read plan from plan file",
				fmt.Sprintf("Cannot read the plan from the given plan file: %s.", err),
			))
			return nil, diags
		}
		if plan.Backend.Config == nil {
			// Should never happen; always indicates a bug in the creation of the plan file
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Failed to read plan from plan file",
				"The given plan file does not have a valid backend configuration. This is a bug in the OpenTofu command that generated this plan file.",
			))
			return nil, diags
		}
		be, beDiags = c.BackendForLocalPlan(ctx, plan.Backend, enc)
	} else {
		// Both new plans and saved cloud plans load their backend from config.
		backendConfig, configDiags := c.loadBackendConfig(ctx, ".")
		diags = diags.Append(configDiags)
		if configDiags.HasErrors() {
			return nil, diags
		}

		be, beDiags = c.Backend(ctx, &BackendOpts{
			Config:      backendConfig,
			ViewOptions: viewOptions,
		}, enc)
	}

	diags = diags.Append(beDiags)
	if beDiags.HasErrors() {
		return nil, diags
	}
	return be, diags
}

func (c *ApplyCommand) OperationRequest(
	ctx context.Context,
	be backend.Enhanced,
	view views.Apply,
	applyArgs *arguments.Apply,
	planFile *planfile.WrappedPlanFile,
	enc encryption.Encryption,
) (*backend.Operation, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Applying changes with dev overrides in effect could make it impossible
	// to switch back to a release version if the schema isn't compatible,
	// so we'll warn about it.
	diags = diags.Append(c.providerDevOverrideRuntimeWarnings())

	// Build the operation
	opReq := c.Operation(ctx, be, applyArgs.ViewOptions, enc)
	opReq.AutoApprove = applyArgs.AutoApprove
	opReq.SuppressForgetErrorsDuringDestroy = applyArgs.SuppressForgetErrorsDuringDestroy
	opReq.ConfigDir = "."
	opReq.PlanMode = applyArgs.Operation.PlanMode
	opReq.Hooks = view.Hooks()
	opReq.PlanFile = planFile
	opReq.PlanRefresh = applyArgs.Operation.Refresh
	opReq.Targets = applyArgs.Operation.Targets
	opReq.Excludes = applyArgs.Operation.Excludes
	opReq.ForceReplace = applyArgs.Operation.ForceReplace
	opReq.Type = backend.OperationTypeApply
	opReq.View = view.Operation()

	var err error
	opReq.ConfigLoader, err = c.initConfigLoader()
	if err != nil {
		diags = diags.Append(fmt.Errorf("Failed to initialize config loader: %w", err))
		return nil, diags
	}

	return opReq, diags
}

func (c *ApplyCommand) GatherVariables(args *arguments.Vars) {
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

func (c *ApplyCommand) Help() string {
	if c.Destroy {
		return c.helpDestroy()
	}

	return c.helpApply()
}

func (c *ApplyCommand) Synopsis() string {
	if c.Destroy {
		return "Destroy previously-created infrastructure"
	}

	return "Create or update infrastructure"
}

func (c *ApplyCommand) helpApply() string {
	helpText := `
Usage: tofu [global options] apply [options] [PLAN]

  Creates or updates infrastructure according to OpenTofu configuration
  files in the current directory.

  By default, OpenTofu will generate a new plan and present it for your
  approval before taking any action. You can optionally provide a plan
  file created by a previous call to "tofu plan", in which case
  OpenTofu will take the actions described in that plan without any
  confirmation prompt.

Options:

  -auto-approve                Skip interactive approval of plan before applying.

  -backup=path                 Path to backup the existing state file before
                               modifying. Defaults to the "-state-out" path with
                               ".backup" extension. Set to "-" to disable backup.

  -compact-warnings            If OpenTofu produces any warnings that are not
                               accompanied by errors, show them in a more compact
                               form that includes only the summary messages.

  -consolidate-warnings=false  If OpenTofu produces any warnings, no consolidation
                               will be performed. All locations, for all warnings
                               will be listed. Enabled by default.

  -consolidate-errors          If OpenTofu produces any errors, no consolidation
                               will be performed. All locations, for all errors
                               will be listed. Disabled by default.

  -destroy                     Destroy OpenTofu-managed infrastructure.
                               The command "tofu destroy" is a convenience alias
                               for this option.

  -lock=false                  Don't hold a state lock during the operation.
                               This is dangerous if others might concurrently
                               run commands against the same workspace.

  -lock-timeout=0s             Duration to retry a state lock.

  -input=true                  Ask for input for variables if not directly set.

  -no-color                    If specified, output won't contain any color.

  -concise                     Disables progress-related messages in the output.

  -parallelism=n               Limit the number of parallel resource operations.
                               Defaults to 10.

  -state=path                  Path to read and save state (unless state-out
                               is specified). Defaults to "terraform.tfstate".

  -state-out=path              Path to write state to that is different than
                               "-state". This can be used to preserve the old
                               state.

  -show-sensitive              If specified, sensitive values will be displayed.

  -suppress-forget-errors      Suppress the error that occurs when a destroy
                               operation completes successfully but leaves
                               forgotten instances behind.

  -var 'foo=bar'               Set a variable in the OpenTofu configuration.
                               This flag can be set multiple times.

  -var-file=foo                Set variables in the OpenTofu configuration from
                               a file.
                               If "terraform.tfvars" or any ".auto.tfvars"
                               files are present, they will be automatically
                               loaded.

  -json                        Produce output in a machine-readable JSON format,
                               suitable for use in text editor integrations and
                               other automated systems. Always disables color.

  -json-into=out.json          Produce the same output as -json, but sent directly
                               to the given file. This allows automation to preserve
                               the original human-readable output streams, while
                               capturing more detailed logs for machine analysis.

  -deprecation=module:m        Specify what type of warnings are shown. Accepted
                               values for "m": all, local, none. Default: all.
                               When "all" is selected, OpenTofu will show the
                               deprecation warnings for all modules. When "local"
                               is selected, the warns will be shown only for the
                               modules that are imported with a relative path.
                               When "none" is selected, all the deprecation
                               warnings will be dropped.

  If you don't provide a saved plan file then this command will also accept
  all of the plan-customization options accepted by the tofu plan command.
  For more information on those options, run:
      tofu plan -help
`
	return strings.TrimSpace(helpText)
}

func (c *ApplyCommand) helpDestroy() string {
	helpText := `
Usage: tofu [global options] destroy [options]

  Destroy OpenTofu-managed infrastructure.

  This command is a convenience alias for:
      tofu apply -destroy

Options:

  -suppress-forget-errors      Suppress the error that occurs when a destroy
                               operation completes successfully but leaves
                               forgotten instances behind.

  This command also accepts many of the plan-customization options accepted by
  the tofu plan command. For more information on those options, run:
      tofu plan -help
`
	return strings.TrimSpace(helpText)
}
