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
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// RefreshCommand is a cli.Command implementation that refreshes the state
// file.
type RefreshCommand struct {
	Meta
}

func (c *RefreshCommand) Run(rawArgs []string) int {
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
	args, diags := arguments.ParseRefresh(rawArgs)

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewRefresh(args.ViewType, c.View)

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

	// FIXME: the -input flag value is needed to initialize the backend and the
	// operation, but there is no clear path to pass this value down, so we
	// continue to mutate the Meta object state for now.
	c.Meta.input = args.InputEnabled

	// FIXME: the -parallelism flag is used to control the concurrency of
	// OpenTofu operations. At the moment, this value is used both to
	// initialize the backend via the ContextOpts field inside CLIOpts, and to
	// set a largely unused field on the Operation request. Again, there is no
	// clear path to pass this value down, so we continue to mutate the Meta
	// object state for now.
	c.Meta.parallelism = args.Operation.Parallelism

	// Inject variables from args into meta for static evaluation
	c.GatherVariables(args.Vars)

	// Load the encryption configuration
	enc, encDiags := c.Encryption(ctx)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	// Prepare the backend with the backend-specific arguments
	be, beDiags := c.PrepareBackend(ctx, args.State, args.ViewType, enc)
	diags = diags.Append(beDiags)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Build the operation request
	opReq, opDiags := c.OperationRequest(ctx, be, view, args.ViewType, args.Operation, enc)
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

func (c *RefreshCommand) PrepareBackend(ctx context.Context, args *arguments.State, viewType arguments.ViewType, enc encryption.Encryption) (backend.Enhanced, tfdiags.Diagnostics) {
	// FIXME: we need to apply the state arguments to the meta object here
	// because they are later used when initializing the backend. Carving a
	// path to pass these arguments to the functions that need them is
	// difficult but would make their use easier to understand.
	c.Meta.applyStateArguments(args)

	backendConfig, diags := c.loadBackendConfig(ctx, ".")
	if diags.HasErrors() {
		return nil, diags
	}

	// Load the backend
	be, beDiags := c.Backend(ctx, &BackendOpts{
		Config:   backendConfig,
		ViewType: viewType,
	}, enc.State())
	diags = diags.Append(beDiags)
	if beDiags.HasErrors() {
		return nil, diags
	}

	return be, diags
}

func (c *RefreshCommand) OperationRequest(ctx context.Context, be backend.Enhanced, view views.Refresh, viewType arguments.ViewType, args *arguments.Operation, enc encryption.Encryption,
) (*backend.Operation, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Build the operation
	opReq := c.Operation(ctx, be, viewType, enc)
	opReq.ConfigDir = "."
	opReq.Hooks = view.Hooks()
	opReq.Targets = args.Targets
	opReq.Excludes = args.Excludes
	opReq.Type = backend.OperationTypeRefresh
	opReq.View = view.Operation()

	var err error
	opReq.ConfigLoader, err = c.initConfigLoader()
	if err != nil {
		diags = diags.Append(fmt.Errorf("Failed to initialize config loader: %w", err))
		return nil, diags
	}

	return opReq, diags
}

func (c *RefreshCommand) GatherVariables(args *arguments.Vars) {
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

func (c *RefreshCommand) Help() string {
	helpText := `
Usage: tofu [global options] refresh [options]

  Update the state file of your infrastructure with metadata that matches
  the physical resources they are tracking.

  This will not modify your infrastructure, but it can modify your
  state file to update metadata. This metadata might cause new changes
  to occur when you generate a plan or call apply next.

Options:

  -compact-warnings      If OpenTofu produces any warnings that are not
                         accompanied by errors, show them in a more compact form
                         that includes only the summary messages.

  -consolidate-warnings  If OpenTofu produces any warnings, no consolidation
                         will be performed. All locations, for all warnings
                         will be listed. Enabled by default.

  -consolidate-errors    If OpenTofu produces any errors, no consolidation
                         will be performed. All locations, for all errors
                         will be listed. Disabled by default

  -exclude=resource      Resource to exclude. Operation will be limited to all
                         resources that are not excluded or dependent on excluded
                         resources. This flag can be used multiple times. Cannot
                         be used alongside the -target flag.

  -input=true            Ask for input for variables if not directly set.

  -lock=false            Don't hold a state lock during the operation. This is
                         dangerous if others might concurrently run commands
                         against the same workspace.

  -lock-timeout=0s       Duration to retry a state lock.

  -no-color              If specified, output won't contain any color.

  -concise               Disables progress-related messages in the output.

  -parallelism=n         Limit the number of concurrent operations. Defaults to 10.

  -target=resource       Resource to target. Operation will be limited to this
                         resource and its dependencies. This flag can be used
                         multiple times.  Cannot be used alongside the -exclude
                         flag.

  -var 'foo=bar'         Set a variable in the OpenTofu configuration. This
                         flag can be set multiple times.

  -var-file=foo          Set variables in the OpenTofu configuration from
                         a file. If "terraform.tfvars" or any ".auto.tfvars"
                         files are present, they will be automatically loaded.

  -json                  Produce output in a machine-readable JSON format,
                         suitable for use in text editor integrations and 
                         other automated systems. Always disables color.

  -state, state-out, and -backup are legacy options supported for the local
  backend only. For more information, see the local backend's documentation.
`
	return strings.TrimSpace(helpText)
}

func (c *RefreshCommand) Synopsis() string {
	return "Update the state to match remote systems"
}
