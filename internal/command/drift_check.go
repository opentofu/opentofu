// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/plans/planfile"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// DriftCheckCommand is a Command implementation that compares a OpenTofu
// configuration to an actual infrastructure and shows the differences.
type DriftCheckCommand struct {
	Meta
}

func (c *DriftCheckCommand) Check(ctx context.Context, rawArgs []string) int {
	fmt.Println("Running Check Again", rawArgs)
	// Parse and validate flags
	args, diags := arguments.ParsePlan(rawArgs)

	c.View.SetShowSensitive(args.ShowSensitive)

	// FIXME: the -input flag value is needed to initialize the backend and the
	// operation, but there is no clear path to pass this value down, so we
	// continue to mutate the Meta object state for now.
	c.Meta.input = args.InputEnabled
	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewDriftCheck(c.View)

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
	// FIXME: the -parallelism flag is used to control the concurrency of
	// OpenTofu operations. At the moment, this value is used both to
	// initialize the backend via the ContextOpts field inside CLIOpts, and to
	// set a largely unused field on the Operation request. Again, there is no
	// clear path to pass this value down, so we continue to mutate the Meta
	// object state for now.
	c.Meta.parallelism = args.Operation.Parallelism

	diags = diags.Append(c.providerDevOverrideRuntimeWarnings())

	// Inject variables from args into meta for static evaluation
	c.GatherVariables(args.Vars)

	// Load the encryption configuration
	enc, encDiags := c.Encryption(ctx)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Prepare the backend, passing the plan file if present, and the
	// backend-specific arguments
	be, beDiags := c.PrepareBackend(ctx, args.State, args.ViewType, enc.State())
	diags = diags.Append(beDiags)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Build the operation request
	opReq, opDiags := c.OperationRequest(ctx, be, view, args.ViewType, args.Operation, enc)
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

	view.Diagnostics(diags)

	if diags.HasErrors() {
		return 1
	}

	return 0
}

func (c *DriftCheckCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()

	// Parse and apply global view arguments
	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)

	// Propagate -no-color for legacy use of Ui.  The remote backend and
	// cloud package use this; it should be removed when/if they are
	// migrated to views.
	c.Meta.color = !common.NoColor
	c.Meta.Color = c.Meta.color

	for {
		fmt.Println("Calculating Drift")
		time.Sleep(1 * time.Second)
		c.Check(ctx, rawArgs)
		time.Sleep(5 * time.Second)
		// If we don't set the configLoader to nil, it's going to
		// reuse the same config from the previous run,
		// so Tofu won't see difference between Check calls
		c.configLoader = nil
	}
	// return 0
}

func (c *DriftCheckCommand) PrepareBackend(ctx context.Context, args *arguments.State, viewType arguments.ViewType, enc encryption.StateEncryption) (backend.Enhanced, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	c.Meta.applyStateArguments(args)

	// Load the backend
	var be backend.Enhanced
	var beDiags tfdiags.Diagnostics
	backendConfig, configDiags := c.loadBackendConfig(ctx, ".")
	diags = diags.Append(configDiags)
	if configDiags.HasErrors() {
		return nil, diags
	}

	be, beDiags = c.Backend(ctx, &BackendOpts{
		Config:   backendConfig,
		ViewType: viewType,
	}, enc)

	diags = diags.Append(beDiags)
	if beDiags.HasErrors() {
		return nil, diags
	}
	return be, diags
}

func (c *DriftCheckCommand) OperationRequest(
	ctx context.Context,
	be backend.Enhanced,
	view views.DriftCheck,
	viewType arguments.ViewType,
	args *arguments.Operation,
	enc encryption.Encryption,
) (*backend.Operation, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Build the operation
	opReq := c.Operation(ctx, be, viewType, enc)
	opReq.ConfigDir = "."
	opReq.PlanMode = args.PlanMode
	opReq.Hooks = view.Hooks()
	opReq.PlanRefresh = args.Refresh
	opReq.Targets = args.Targets
	opReq.Excludes = args.Excludes
	opReq.ForceReplace = args.ForceReplace
	opReq.Type = backend.OperationTypePlan
	opReq.View = view.Operation()

	var err error
	opReq.ConfigLoader, err = c.initConfigLoader()
	if err != nil {
		diags = diags.Append(fmt.Errorf("Failed to initialize config loader: %w", err))
		return nil, diags
	}

	return opReq, diags
}

func (c *DriftCheckCommand) GatherVariables(args *arguments.Vars) {
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

func (c *DriftCheckCommand) Help() string {
	helpText := `
Usage: tofu [global options] plan [options]

  Generates a speculative execution plan, showing what actions OpenTofu would
  take to apply the current configuration. This command will not actually
  perform the planned actions.

  You can optionally save the plan to a file, which you can then pass to the
  "apply" command to perform exactly the actions described in the plan.

Plan Customization Options:

  The following options customize how OpenTofu will produce its plan. You can
  also use these options when you run "tofu apply" without passing it a saved
  plan, in order to plan and apply in a single command.

  -destroy                Select the "destroy" planning mode, which creates a
                          plan to destroy all objects currently managed by this
                          OpenTofu configuration instead of the usual behavior.

  -refresh-only           Select the "refresh only" planning mode, which checks
                          whether remote objects still match the outcome of the
                          most recent OpenTofu apply but does not propose any
                          actions to undo any changes made outside of OpenTofu.

  -refresh=false          Skip checking for external changes to remote objects
                          while creating the plan. This can potentially make
                          planning faster, but at the expense of possibly
                          planning against a stale record of the remote system
                          state.

  -replace=resource       Force replacement of a particular resource instance
                          using its resource address. If the plan would've
                          otherwise produced an update or no-op action for this
                          instance, OpenTofu will plan to replace it instead.
                          You can use this option multiple times to replace
                          more than one object.

  -target=resource        Limit the planning operation to only the given
                          module, resource, or resource instance and all of its
                          dependencies. You can use this option multiple times
                          to include more than one object. This is for
                          exceptional use only. Cannot be used alongside the
                          -exclude option.

  -target-file=filename   Similar to -target, but specifies zero or more
                          resource addresses from a file.

  -exclude=resource       Limit the planning operation to not operate on the
                          given module, resource, or resource instance and all
                          of the resources and modules that depend on it. You
                          can use this option multiple times to exclude more
                          than one object. This is for exceptional use only.
                          Cannot be used together with the -target option.

  -exclude-file=filename  Similar to -exclude, but specifies zero or more
                          resource addresses from a file.

  -var 'foo=bar'          Set a value for one of the input variables in the
                          root module of the configuration. Use this option
                          more than once to set more than one variable.

  -var-file=filename      Load variable values from the given file, in addition
                          to the default files terraform.tfvars and
                          *.auto.tfvars. Use this option more than once to
                          include more than one variables file.

Other Options:

  -compact-warnings            If OpenTofu produces any warnings that are not
                               accompanied by errors, shows them in a more
                               compact form that includes only the summary
                               messages.

  -consolidate-warnings=false  If OpenTofu produces any warnings, do not
                               attempt to consolidate similar messages. All
                               locations for all warnings will be listed.

  -consolidate-errors          If OpenTofu produces any errors, attempt to
                               consolidate similar messages into a single item.

  -detailed-exitcode           Return detailed exit codes when the command
                               exits. The detailed exit codes are:
                                 0 - Succeeded but no changes proposed
                                 1 - Planning failed with an error
                                 2 - Succeeded and changes are proposed

  -generate-config-out=path    (Experimental) If import blocks are present in
                               configuration, instructs OpenTofu to generate
                               HCL for any imported resources not already
                               present. The configuration is written to a new
                               file at PATH, which must not already exist.
                               OpenTofu may still attempt to write
                               configuration if planning fails with an error.

  -input=false                 Disable prompting for required input variables
                               that are not set some other way.

  -lock=false                  Don't hold a state lock during the operation.
                               This is dangerous if others might concurrently
                               run commands against the same workspace.

  -lock-timeout=duration       Duration to retry a state lock, such as "5s"
                               to represent five seconds.

  -no-color                    Disable virtual terminal escape sequences.

  -concise                     Disable progress-related messages.

  -out=path                    Write a plan file to the given path. This can be
                               used as input to the "apply" command.

  -parallelism=n               Limit the number of concurrent operations.
                               Defaults to 10.

  -state=statefile             A legacy option used for the local backend only.
                               Refer to the local backend's documentation for
                               more information.

  -show-sensitive              If specified, sensitive values will not be
                               redacted in te UI output.

  -json                        Produce output in a machine-readable JSON
                               format, suitable for use in text editor
                               integrations and other automated systems.

  -deprecation=module:m        Specify what type of warnings are shown.
                               Accepted values for "m": all, local, none. 
                               Default: all. When "all" is selected, OpenTofu
                               will show the deprecation warnings for all
                               modules. When "local" is selected, the warns
                               will be shown only for the modules that are
                               imported with a relative path. When "none" is
                               selected, all the deprecation warnings will be
                               dropped.
`
	return strings.TrimSpace(helpText)
}

func (c *DriftCheckCommand) Synopsis() string {
	return "Show changes required by the current configuration"
}

func (c *DriftCheckCommand) LoadPlanFile(path string, enc encryption.Encryption) (*planfile.WrappedPlanFile, tfdiags.Diagnostics) {
	var planFile *planfile.WrappedPlanFile
	var diags tfdiags.Diagnostics

	// Try to load plan if path is specified
	if path != "" {
		var err error
		planFile, err = c.PlanFile(path, enc.Plan())
		fmt.Println("LoadPlanFile.PlanFile", planFile)
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
	}

	return planFile, diags
}
