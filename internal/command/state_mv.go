// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/flags"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/clistate"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

// StateMvCommand is a Command implementation that shows a single resource.
type StateMvCommand struct {
	StateMeta
}

func (c *StateMvCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()

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
	args, closer, diags := arguments.ParseStateMv(rawArgs)
	defer closer()

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewState(args.ViewOptions, c.View)
	// ... and initialise the Meta.Ui to wrap Meta.View into a new implementation
	// that is able to print by using View abstraction and use the Meta.Ui
	// to ask for the user input.
	c.Meta.configureUiFromView(args.ViewOptions)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		if args.ViewOptions.ViewType == arguments.ViewJSON {
			return 1 // in case it's json, do not print the help of the command
		}
		return cli.RunResultHelp
	}
	// TODO meta-refactor: remove these assignments once there is a clear way to propagate these to the place
	//   where are used
	c.backupPath = args.BackupPath
	c.statePath = args.StatePath
	c.stateLock = args.Backend.StateLock
	c.stateLockTimeout = args.Backend.StateLockTimeout
	c.ignoreRemoteVersion = args.Backend.IgnoreRemoteVersion
	c.GatherVariables(args.Vars)

	if diags := c.Meta.checkRequiredVersion(ctx); diags != nil {
		view.Diagnostics(diags)
		return 1
	}

	// If backup or backup-out options are set
	// and the state option is not set, make sure
	// the backend is local
	backupOptionSetWithoutStateOption := args.BackupPath != "-" && args.StatePath == ""
	backupOutOptionSetWithoutStateOption := args.BackupPathOut != "-" && args.StatePath == ""

	var setLegacyLocalBackendOptions []string
	if backupOptionSetWithoutStateOption {
		setLegacyLocalBackendOptions = append(setLegacyLocalBackendOptions, "-backup")
	}
	if backupOutOptionSetWithoutStateOption {
		setLegacyLocalBackendOptions = append(setLegacyLocalBackendOptions, "-backup-out")
	}

	// Load the encryption configuration
	enc, encDiags := c.Encryption(ctx)
	if encDiags.HasErrors() {
		view.Diagnostics(encDiags)
		return 1
	}

	if len(setLegacyLocalBackendOptions) > 0 {
		currentBackend, diags := c.backendFromConfig(ctx, &BackendOpts{ViewOptions: args.ViewOptions}, enc.State())
		if diags.HasErrors() {
			view.Diagnostics(diags)
			return 1
		}

		// If currentBackend is nil and diags didn't have errors,
		// this means we have an implicit local backend
		_, isLocalBackend := currentBackend.(backend.Local)
		if currentBackend != nil && !isLocalBackend {
			diags = diags.Append(
				tfdiags.Sourceless(
					tfdiags.Error,
					fmt.Sprintf("Invalid command line options: %s", strings.Join(setLegacyLocalBackendOptions[:], ", ")),
					"Command line options -backup and -backup-out are legacy options that operate on a local state file only. You must specify a local state file with the -state option or switch to the local backend.",
				),
			)
			view.Diagnostics(diags)
			return 1
		}
	}

	// Read the from state
	stateFromMgr, err := c.State(ctx, enc, view, args.ViewOptions)
	if err != nil {
		view.StateLoadingFailure(err.Error())
		return 1
	}

	if c.stateLock {
		stateLocker := clistate.NewLocker(c.stateLockTimeout, views.NewStateLocker(args.ViewOptions, c.View))
		if diags := stateLocker.Lock(stateFromMgr, "state-mv"); diags.HasErrors() {
			view.Diagnostics(diags)
			return 1
		}
		defer func() {
			if diags := stateLocker.Unlock(); diags.HasErrors() {
				view.Diagnostics(diags)
			}
		}()
	}

	if err := stateFromMgr.RefreshState(context.TODO()); err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to refresh source state",
			err.Error(),
		)))
		return 1
	}

	stateFrom := stateFromMgr.State()
	if stateFrom == nil {
		view.StateNotFound()
		return 1
	}

	// Read the destination state
	stateToMgr := stateFromMgr
	stateTo := stateFrom

	if args.StateOutPath != "" {
		c.statePath = args.StateOutPath
		c.backupPath = args.BackupPathOut

		stateToMgr, err = c.State(ctx, enc, view, args.ViewOptions)
		if err != nil {
			view.StateLoadingFailure(err.Error())
			return 1
		}

		if c.stateLock {
			stateLocker := clistate.NewLocker(c.stateLockTimeout, views.NewStateLocker(args.ViewOptions, c.View))
			if diags := stateLocker.Lock(stateToMgr, "state-mv"); diags.HasErrors() {
				view.Diagnostics(diags)
				return 1
			}
			defer func() {
				if diags := stateLocker.Unlock(); diags.HasErrors() {
					view.Diagnostics(diags)
				}
			}()
		}

		if err := stateToMgr.RefreshState(context.TODO()); err != nil {
			view.Diagnostics(diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Failed to refresh destination state",
				err.Error(),
			)))
			return 1
		}

		stateTo = stateToMgr.State()
		if stateTo == nil {
			stateTo = states.NewState()
		}
	}

	sourceAddr, moreDiags := c.lookupSingleStateObjectAddr(stateFrom, args.RawSrcAddr)
	diags = diags.Append(moreDiags)
	destAddr, moreDiags := c.lookupSingleStateObjectAddr(stateFrom, args.RawDestAddr)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	const msgInvalidSource = "Invalid source address"
	const msgInvalidTarget = "Invalid target address"

	var moved int
	ssFrom := stateFrom.SyncWrapper()
	sourceAddrs := c.sourceObjectAddrs(stateFrom, sourceAddr)
	if len(sourceAddrs) == 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			msgInvalidSource,
			fmt.Sprintf("Cannot move %s: does not match anything in the current state.", sourceAddr),
		))
		view.Diagnostics(diags)
		return 1
	}
	for _, rawAddrFrom := range sourceAddrs {
		switch addrFrom := rawAddrFrom.(type) {
		case addrs.ModuleInstance:
			search := sourceAddr.(addrs.ModuleInstance)
			addrTo, ok := destAddr.(addrs.ModuleInstance)
			if !ok {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					msgInvalidTarget,
					fmt.Sprintf("Cannot move %s to %s: the target must also be a module.", addrFrom, destAddr),
				))
				view.Diagnostics(diags)
				return 1
			}

			if len(search) < len(addrFrom) {
				n := make(addrs.ModuleInstance, 0, len(addrTo)+len(addrFrom)-len(search))
				n = append(n, addrTo...)
				n = append(n, addrFrom[len(search):]...)
				addrTo = n
			}

			if stateTo.Module(addrTo) != nil {
				view.ErrorMovingToAlreadyExistingDst()
				return 1
			}

			ms := ssFrom.Module(addrFrom)
			if ms == nil {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					msgInvalidSource,
					fmt.Sprintf("The current state does not contain %s.", addrFrom),
				))
				view.Diagnostics(diags)
				return 1
			}

			moved++
			view.ResourceMoveStatus(args.DryRun, addrFrom.String(), addrTo.String())
			if !args.DryRun {
				ssFrom.RemoveModule(addrFrom)

				// Update the address before adding it to the state.
				ms.Addr = addrTo
				stateTo.Modules[addrTo.String()] = ms
			}

		case addrs.AbsResource:
			addrTo, ok := destAddr.(addrs.AbsResource)
			if !ok {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					msgInvalidTarget,
					fmt.Sprintf("Cannot move %s to %s: the source is a whole resource (not a resource instance) so the target must also be a whole resource.", addrFrom, destAddr),
				))
				view.Diagnostics(diags)
				return 1
			}
			diags = diags.Append(c.validateResourceMove(addrFrom, addrTo))

			if stateTo.Resource(addrTo) != nil {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					msgInvalidTarget,
					fmt.Sprintf("Cannot move to %s: there is already a resource at that address in the current state.", addrTo),
				))
			}

			rs := ssFrom.Resource(addrFrom)
			if rs == nil {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					msgInvalidSource,
					fmt.Sprintf("The current state does not contain %s.", addrFrom),
				))
			}

			if diags.HasErrors() {
				view.Diagnostics(diags)
				return 1
			}

			moved++
			view.ResourceMoveStatus(args.DryRun, addrFrom.String(), addrTo.String())
			if !args.DryRun {
				ssFrom.RemoveResource(addrFrom)

				// Update the address before adding it to the state.
				rs.Addr = addrTo
				stateTo.EnsureModule(addrTo.Module).Resources[addrTo.Resource.String()] = rs
			}

		case addrs.AbsResourceInstance:
			addrTo, ok := destAddr.(addrs.AbsResourceInstance)
			if !ok {
				ra, ok := destAddr.(addrs.AbsResource)
				if !ok {
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						msgInvalidTarget,
						fmt.Sprintf("Cannot move %s to %s: the target must also be a resource instance.", addrFrom, destAddr),
					))
					view.Diagnostics(diags)
					return 1
				}
				addrTo = ra.Instance(addrs.NoKey)
			}

			diags = diags.Append(c.validateResourceMove(addrFrom.ContainingResource(), addrTo.ContainingResource()))

			if stateTo.Module(addrTo.Module) == nil {
				// moving something to a mew module, so we need to ensure it exists
				stateTo.EnsureModule(addrTo.Module)
			}
			if stateTo.ResourceInstance(addrTo) != nil {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					msgInvalidTarget,
					fmt.Sprintf("Cannot move to %s: there is already a resource instance at that address in the current state.", addrTo),
				))
			}

			is := ssFrom.ResourceInstance(addrFrom)
			if is == nil {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					msgInvalidSource,
					fmt.Sprintf("The current state does not contain %s.", addrFrom),
				))
			}

			if diags.HasErrors() {
				view.Diagnostics(diags)
				return 1
			}

			moved++
			view.ResourceMoveStatus(args.DryRun, addrFrom.String(), args.RawDestAddr)
			if !args.DryRun {
				fromResourceAddr := addrFrom.ContainingResource()
				fromResource := ssFrom.Resource(fromResourceAddr)
				fromProviderAddr := fromResource.ProviderConfig
				ssFrom.ForgetResourceInstanceAll(addrFrom)
				ssFrom.RemoveResourceIfEmpty(fromResourceAddr)

				rs := stateTo.Resource(addrTo.ContainingResource())
				if rs == nil {
					// If we're moving to an address without an index then that
					// suggests the user's intent is to establish both the
					// resource and the instance at the same time (since the
					// address covers both). If there's an index in the
					// target then allow creating the new instance here.
					resourceAddr := addrTo.ContainingResource()
					stateTo.SyncWrapper().SetResourceProvider(
						resourceAddr,
						fromProviderAddr, // in this case, we bring the provider along as if we were moving the whole resource
					)
					rs = stateTo.Resource(resourceAddr)
				}

				rs.Instances[addrTo.Resource.Key] = is
			}
		default:
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				msgInvalidSource,
				fmt.Sprintf("Cannot move %s: OpenTofu doesn't know how to move this object.", rawAddrFrom),
			))
		}

		// Look for any dependencies that may be effected and
		// remove them to ensure they are recreated in full.
		for _, mod := range stateTo.Modules {
			for _, res := range mod.Resources {
				for _, ins := range res.Instances {
					if ins.Current == nil {
						continue
					}

					for _, dep := range ins.Current.Dependencies {
						// check both directions here, since we may be moving
						// an instance which is in a resource, or a module
						// which can contain a resource.
						if dep.TargetContains(rawAddrFrom) || rawAddrFrom.TargetContains(dep) {
							ins.Current.Dependencies = nil
							break
						}
					}
				}
			}
		}
	}

	if args.DryRun {
		view.DryRunMovedStatus(moved)
		return 0 // This is as far as we go in dry-run mode
	}

	b, backendDiags := c.Backend(ctx, nil, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Get schemas, if possible, before writing state
	var schemas *tofu.Schemas
	if isCloudMode(b) {
		var schemaDiags tfdiags.Diagnostics
		schemas, schemaDiags = c.MaybeGetSchemas(ctx, stateTo, nil)
		diags = diags.Append(schemaDiags)
	}

	// Write the new state
	if err := stateToMgr.WriteState(stateTo); err != nil {
		view.StateSavingError(err.Error())
		return 1
	}
	if err := stateToMgr.PersistState(context.TODO(), schemas); err != nil {
		view.StateSavingError(err.Error())
		return 1
	}

	// Write the old state if it is different
	if stateTo != stateFrom {
		if err := stateFromMgr.WriteState(stateFrom); err != nil {
			view.StateSavingError(err.Error())
			return 1
		}
		if err := stateFromMgr.PersistState(context.TODO(), schemas); err != nil {
			view.StateSavingError(err.Error())
			return 1
		}
	}

	view.Diagnostics(diags)
	view.MoveFinalStatus(moved)
	return 0
}

// sourceObjectAddrs takes a single source object address and expands it to
// potentially multiple objects that need to be handled within it.
//
// In particular, this handles the case where a module is requested directly:
// if it has any child modules, then they must also be moved. It also resolves
// the ambiguity that an index-less resource address could either be a resource
// address or a resource instance address, by making a decision about which
// is intended based on the current state of the resource in question.
func (c *StateMvCommand) sourceObjectAddrs(state *states.State, matched addrs.Targetable) []addrs.Targetable {
	var ret []addrs.Targetable

	switch addr := matched.(type) {
	case addrs.ModuleInstance:
		for _, mod := range state.Modules {
			if len(mod.Addr) < len(addr) {
				continue // can't possibly be our selection or a child of it
			}
			if !mod.Addr[:len(addr)].Equal(addr) {
				continue
			}
			ret = append(ret, mod.Addr)
		}
	case addrs.AbsResource:
		// If this refers to a resource without "count" or "for_each" set then
		// we'll assume the user intended it to be a resource instance
		// address instead, to allow for requests like this:
		//   tofu state mv aws_instance.foo aws_instance.bar[1]
		// That wouldn't be allowed if aws_instance.foo had multiple instances
		// since we can't move multiple instances into one.
		if rs := state.Resource(addr); rs != nil {
			if _, ok := rs.Instances[addrs.NoKey]; ok {
				ret = append(ret, addr.Instance(addrs.NoKey))
			} else {
				ret = append(ret, addr)
			}
		}
	default:
		ret = append(ret, matched)
	}

	return ret
}

func (c *StateMvCommand) validateResourceMove(addrFrom, addrTo addrs.AbsResource) tfdiags.Diagnostics {
	const msgInvalidRequest = "Invalid state move request"
	var diags tfdiags.Diagnostics

	if addrFrom.Resource.Mode == addrs.EphemeralResourceMode || addrTo.Resource.Mode == addrs.EphemeralResourceMode {
		diags = diags.Append(
			tfdiags.Sourceless(
				tfdiags.Error,
				msgInvalidRequest,
				"Ephemeral resources cannot be used as sources or targets for the move action. Just update your configuration accordingly.",
			),
		)
		return diags
	}

	if addrFrom.Resource.Mode != addrTo.Resource.Mode {
		switch addrFrom.Resource.Mode {
		case addrs.ManagedResourceMode:
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				msgInvalidRequest,
				fmt.Sprintf("Cannot move %s to %s: a managed resource can be moved only to another managed resource address.", addrFrom, addrTo),
			))
		case addrs.DataResourceMode:
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				msgInvalidRequest,
				fmt.Sprintf("Cannot move %s to %s: a data resource can be moved only to another data resource address.", addrFrom, addrTo),
			))
			// NOTE: No need for the ephemeral resource in this switch block since it is handled at the top of the method.
		default:
			// In case a new mode is added in future, this unhelpful error is better than nothing.
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				msgInvalidRequest,
				fmt.Sprintf("Cannot move %s to %s: cannot change resource mode.", addrFrom, addrTo),
			))
		}
	}
	if addrFrom.Resource.Type != addrTo.Resource.Type {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			msgInvalidRequest,
			fmt.Sprintf("Cannot move %s to %s: resource types don't match.", addrFrom, addrTo),
		))
	}
	return diags
}

func (c *StateMvCommand) Help() string {
	helpText := `
Usage: tofu [global options] state (move|mv) [options] SOURCE DESTINATION

 This command will move an item matched by the address given to the
 destination address. This command can also move to a destination address
 in a completely different state file.

 This can be used for simple resource renaming, moving items to and from
 a module, moving entire modules, and more. And because this command can also
 move data to a completely new state, it can also be used for refactoring
 one configuration into multiple separately managed OpenTofu configurations.

 This command will output a backup copy of the state prior to saving any
 changes. The backup cannot be disabled. Due to the destructive nature
 of this command, backups are required.

 If you're moving an item to a different state file, a backup will be created
 for each state file.

Options:

  -dry-run                If set, prints out what would've been moved but doesn't
                          actually move anything.

  -lock=false             Don't hold a state lock during the operation. This is
                          dangerous if others might concurrently run commands
                          against the same workspace.

  -lock-timeout=0s        Duration to retry a state lock.

  -ignore-remote-version  A rare option used for the remote backend only. See
                          the remote backend documentation for more information.

  -var 'foo=bar'          Set a value for one of the input variables in the root
                          module of the configuration. Use this option more than
                          once to set more than one variable.

  -var-file=filename      Load variable values from the given file, in addition
                          to the default files terraform.tfvars and *.auto.tfvars.
                          Use this option more than once to include more than one
                          variables file.

  -json                   Produce output in a machine-readable JSON format, 
                          suitable for use in text editor integrations and other 
                          automated systems. Always disables color.

  -json-into=out.json     Produce the same output as -json, but sent directly
                          to the given file. This allows automation to preserve
                          the original human-readable output streams, while
                          capturing more detailed logs for machine analysis.

  -state, state-out, and -backup are legacy options supported for the local
  backend only. For more information, see the local backend's documentation.

`
	return strings.TrimSpace(helpText)
}

func (c *StateMvCommand) Synopsis() string {
	return "Move an item in the state"
}

// TODO meta-refactor: move this to arguments once all commands are using the same shim logic
func (c *StateMvCommand) GatherVariables(args *arguments.Vars) {
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
