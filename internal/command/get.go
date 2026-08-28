// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tracing"
)

func GetCommander() Command {
	cmd := Command{
		Name:  "get",
		Short: "Install or upgrade remote OpenTofu modules",
		Long: `Downloads and installs modules needed for the configuration in the current working directory.

This recursively downloads all modules needed, such as modules imported by modules imported by the root and so on. If a module is already downloaded, it will not be redownloaded or checked for updates unless the -update flag is specified.

Module installation also happens automatically by default as part of the "tofu init" command, so you should rarely need to run this command separately.`,

		DiagsWithNewline: true,
	}

	args := arguments.BindGet(&cmd.CommandLine)
	cmd.Run = func(meta Meta) int {
		return GetCommand{meta}.Execute(args, views.NewGet(args.View, meta.View))
	}

	return cmd
}

// GetCommand is a Command implementation that takes a OpenTofu
// configuration and downloads all the modules.
type GetCommand struct {
	Meta
}

func (c GetCommand) Execute(args *arguments.Get, view views.Get) int {
	ctx := c.CommandContext()
	ctx, span := tracing.Tracer().Start(ctx, "Get")
	defer span.End()

	// Initialization can be aborted by interruption signals
	ctx, done := c.InterruptibleContext(ctx)
	defer done()

	// This gets the current directory as full path.
	path := c.WorkingDir.NormalizePath(c.WorkingDir.RootModuleDir())

	abort, diags := getModules(ctx, &c.Meta, path, args.TestsDirectory, args.Update, view)
	view.Diagnostics(diags)
	if abort || diags.HasErrors() {
		return 1
	}

	return 0
}

func getModules(ctx context.Context, m *Meta, path string, testsDir string, upgrade bool, view views.Get) (abort bool, diags tfdiags.Diagnostics) {
	hooks := view.Hooks(true)
	return m.installModules(ctx, path, testsDir, upgrade, true, hooks, view)
}
