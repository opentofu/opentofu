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
	"github.com/opentofu/opentofu/internal/tfdiags"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states"
)

func StateListCommander() Command {
	cmd := Command{
		Name:    "list",
		Aliases: []string{"ls"},
		Short:   "List resources in the state",
		Long: `List resources in the OpenTofu state.

This command lists resource instances in the OpenTofu state. The address argument can be used to filter the instances by resource or module. If no pattern is given, all resource instances are listed.

The addresses must either be module addresses or absolute resource addresses, such as:
    aws_instance.example
    module.example
    module.example.module.child
    module.example.aws_instance.example

An error will be returned if any of the resources or modules given as filter addresses do not exist in the state.`,

		DiagsWithNewline: true,
	}

	args := arguments.BindStateList(&cmd.CommandLine)
	cmd.Run = func(meta Meta) int {
		return StateListCommand{StateMeta{meta}}.Execute(args, views.NewState(args.View, meta.View))
	}

	return cmd
}

// StateListCommand is a Command implementation that lists the resources
// within a state file.
type StateListCommand struct {
	StateMeta
}

func (c StateListCommand) Execute(args *arguments.StateList, view views.State) int {
	var diags tfdiags.Diagnostics

	ctx := c.CommandContext()

	// Load the encryption configuration
	enc, encDiags := c.Encryption(ctx)
	if encDiags.HasErrors() {
		view.Diagnostics(encDiags)
		return 1
	}

	// Load the backend
	b, backendDiags := c.Backend(ctx, nil, enc.State())
	if backendDiags.HasErrors() {
		view.Diagnostics(backendDiags)
		return 1
	}

	// This is a read-only command
	c.ignoreRemoteVersionConflict(b)

	// Get the state
	env, err := c.Workspace(ctx)
	if err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error selecting workspace",
			err.Error(),
		)))
		return 1
	}
	stateMgr, err := b.StateMgr(ctx, env)
	if err != nil {
		view.StateLoadingFailure(err.Error())
		return 1
	}
	if err := stateMgr.RefreshState(context.TODO()); err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error refreshing the state",
			fmt.Sprintf("Failed to load state: %s", err),
		)))
		return 1
	}

	state := stateMgr.State()
	if state == nil {
		view.StateNotFound()
		return 1
	}

	var resourceAddrs []addrs.AbsResourceInstance
	if len(args.InstancesRawAddr) == 0 {
		resourceAddrs, diags = c.lookupAllResourceInstanceAddrs(state)
	} else {
		resourceAddrs, diags = c.lookupResourceInstanceAddrs(state, args.InstancesRawAddr...)
	}
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	for _, addr := range resourceAddrs {
		if is := state.ResourceInstance(addr); is != nil {
			if args.LookupId == "" || args.LookupId == states.LegacyInstanceObjectID(is.Current) {
				view.StateListAddr(addr)
			}
		}
	}

	view.Diagnostics(diags)

	return 0
}
