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
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tracing"

	"github.com/opentofu/opentofu/internal/tofu"
)

func UnlockCommander() Command {
	cmd := Command{
		Name:  "force-unlock",
		Short: "Release a stuck lock on the current workspace",
		Long: `Manually unlock the state for the defined configuration.

This will not modify your infrastructure. This command removes the lock on the state for the current workspace. The behavior of this lock is dependent on the backend being used. Local state files cannot be unlocked by another process.`,

		DiagsWithNewline: true,
	}

	args := arguments.BindUnlock(&cmd.CommandLine)
	cmd.Run = func(meta Meta) int {
		return UnlockCommand{meta}.Execute(args, views.NewUnlock(args.View, meta.View))
	}

	return cmd
}

// UnlockCommand is a cli.Command implementation that manually unlocks
// the state.
type UnlockCommand struct {
	Meta
}

func (c UnlockCommand) Execute(args *arguments.Unlock, view views.Unlock) int {
	var diags tfdiags.Diagnostics

	ctx := c.CommandContext()
	ctx, span := tracing.Tracer().Start(ctx, "Unlock")
	defer span.End()

	lockID := args.LockID

	// This gets the current directory as full path.
	configPath := c.WorkingDir.NormalizePath(c.WorkingDir.RootModuleDir())

	// Load the encryption configuration
	enc, encDiags := c.EncryptionFromPath(ctx, configPath)
	if encDiags.HasErrors() {
		view.Diagnostics(encDiags)
		return 1
	}

	backendConfig, backendDiags := c.loadBackendConfig(ctx, configPath)
	diags = diags.Append(backendDiags)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Load the backend
	b, backendDiags := c.Backend(ctx, &BackendOpts{
		Config: backendConfig,
		View:   view.Backend(),
	}, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// unlocking is read only when looking at state data
	c.ignoreRemoteVersionConflict(b)

	env, err := c.Workspace(ctx)
	if err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Error selecting workspace: %s", err)))
		return 1
	}
	stateMgr, err := b.StateMgr(ctx, env)
	if err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Failed to load state: %s", err)))
		return 1
	}

	_, isLocal := stateMgr.(*statemgr.Filesystem)

	if optionalLocker, ok := stateMgr.(statemgr.OptionalLocker); ok {
		// Now we can safely call IsLockingEnabled() on optionalLocker
		if !optionalLocker.IsLockingEnabled() {
			view.LockingDisabledForBackend()
			return 1
		}
	}

	// Proceed with unlocking logic if locking is enabled
	if !args.Force {
		// Forcing this doesn't do anything, but doesn't break anything either,
		// and allows us to run the basic command test too.
		if isLocal {
			view.CannotUnlockByAnotherProcess()
			return 1
		}

		desc := "OpenTofu will remove the lock on the remote state.\n" +
			"This will allow local OpenTofu commands to modify this state, even though it\n" +
			"may still be in use. Only 'yes' will be accepted to confirm."

		v, err := c.UIInput().Input(context.Background(), &tofu.InputOpts{
			Id:          "force-unlock",
			Query:       "Do you really want to force-unlock?",
			Description: desc,
		})
		if err != nil {
			view.Diagnostics(diags.Append(fmt.Errorf("Error asking for confirmation: %s", err)))
			return 1
		}
		if v != "yes" {
			view.ForceUnlockCancelled()
			return 1
		}
	}

	if err := stateMgr.Unlock(context.TODO(), lockID); err != nil {
		view.Diagnostics(diags.Append(fmt.Errorf("Failed to unlock state: %s", err)))
		return 1
	}
	view.ForceUnlockSucceeded()
	return 0
}
