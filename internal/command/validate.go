// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

func ValidateCommander() Command {
	cmd := Command{
		Name:  "validate",
		Short: "Check whether the configuration is valid",
		Long: `Validate the configuration files in a directory, referring only to the configuration and not accessing any remote services such as remote state, provider APIs, etc.

Validate runs checks that verify whether a configuration is syntactically valid and internally consistent, regardless of any provided variables or existing state. It is thus primarily useful for general verification of reusable modules, including correctness of attribute names and value types.

It is safe to run this command automatically, for example as a post-save check in a text editor or as a test step for a re-usable module in a CI
system.

Validation requires an initialized working directory with any referenced plugins and modules installed. To initialize a working directory for validation without accessing any configured remote backend, use:
    tofu init -backend=false

To verify configuration in the context of a particular run (a particular target workspace, input variable values, etc), use the 'tofu plan' command instead, which includes an implied validation check.`,

		GroupID: MainCommandGroup.ID,
	}

	args := arguments.BindValidate(&cmd.CommandLine)
	cmd.Run = func(meta Meta) int {
		return ValidateCommand{meta}.Execute(args, views.NewValidate(args.View, meta.View))
	}

	return cmd
}

// ValidateCommand is a Command implementation that validates the tofu files
type ValidateCommand struct {
	Meta
}

func (c ValidateCommand) Execute(args *arguments.Validate, view views.Validate) int {
	var diags tfdiags.Diagnostics
	ctx := c.CommandContext()
	ctx = tfdiags.ContextWithLintFilterHints(ctx, args.View.LintInclude, args.View.LintExclude)
	diags = diags.Append(tfdiags.ExperimentalLintWarn(ctx))

	// After this point, we must only produce JSON output if JSON mode is
	// enabled, so all errors should be accumulated into diags and we'll
	// print out a suitable result at the end, depending on the format
	// selection. All returns from this point on must be tail-calls into
	// view.Results in order to produce the expected output.

	dir, err := filepath.Abs(args.Path)
	if err != nil {
		diags = diags.Append(fmt.Errorf("unable to locate module: %w", err))
		return view.Results(diags)
	}

	// Check for user-supplied plugin path
	if c.pluginPath, err = c.loadPluginPath(); err != nil {
		diags = diags.Append(fmt.Errorf("error loading plugin path: %w", err))
		return view.Results(diags)
	}

	validateDiags := c.validate(ctx, dir, args.TestDirectory, args.NoTests)
	diags = diags.Append(validateDiags)

	// Validating with dev overrides in effect means that the result might
	// not be valid for a stable release, so we'll warn about that in case
	// the user is trying to use "tofu validate" as a sort of pre-flight
	// check before submitting a change.
	diags = diags.Append(c.providerDevOverrideRuntimeWarnings())

	return view.Results(diags)
}

func (c *ValidateCommand) validate(ctx context.Context, dir, testDir string, noTests bool) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	var cfg *configs.Config

	if noTests {
		cfg, diags = c.loadConfig(ctx, dir)
	} else {
		cfg, diags = c.loadConfigWithTests(ctx, dir, testDir)
	}
	if diags.HasErrors() {
		return diags
	}

	validate := func(cfg *configs.Config) tfdiags.Diagnostics {
		var diags tfdiags.Diagnostics

		opts, err := c.contextOpts(ctx)
		if err != nil {
			diags = diags.Append(err)
			return diags
		}

		tfCtx, ctxDiags := tofu.NewContext(opts)
		diags = diags.Append(ctxDiags)
		if ctxDiags.HasErrors() {
			return diags
		}

		return diags.Append(tfCtx.Validate(ctx, cfg))
	}

	diags = diags.Append(validate(cfg))

	if noTests {
		return diags
	}

	validatedModules := make(map[string]bool)

	// We'll also do a quick validation of the OpenTofu test files. These live
	// outside the OpenTofu graph so we have to do this separately.
	for _, file := range cfg.Module.Tests {

		diags = diags.Append(file.Validate())

		for _, run := range file.Runs {

			if run.Module != nil {
				// Then we can also validate the referenced modules, but we are
				// only going to do this is if they are local modules.
				//
				// Basically, local testing modules are something the user can
				// reasonably go and fix. If it's a module being downloaded from
				// the registry, the expectation is that the author of the
				// module should have ran `tofu validate` themselves.
				if _, ok := run.Module.Source.(addrs.ModuleSourceLocal); ok {

					if validated := validatedModules[run.Module.Source.String()]; !validated {

						// Since we can reference the same module twice, let's
						// not validate the same thing multiple times.

						validatedModules[run.Module.Source.String()] = true
						diags = diags.Append(validate(run.ConfigUnderTest))
					}

				}
			}

			diags = diags.Append(run.Validate())
		}
	}

	return diags
}
