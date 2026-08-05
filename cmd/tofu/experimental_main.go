// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/hashicorp/go-plugin"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command"
	"github.com/opentofu/opentofu/internal/command/cliconfig"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/command/workdir"
	"github.com/opentofu/opentofu/internal/didyoumean"
	"github.com/opentofu/opentofu/internal/getmodules"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/svchost/disco"

	cli "github.com/urfave/cli/v3"
)

func experimentalMain(
	ctx context.Context,
	view *views.View,
	rv *views.Root,
	config *cliconfig.Config,
	services *disco.Disco,
	modulePkgFetcher *getmodules.PackageFetcher,
	providerDevOverrides map[addrs.Provider]getproviders.PackageLocalDir,
	unmanagedProviders map[addrs.Provider]*plugin.ReattachConfig,
) int {
	// Patch HelpPrinter to include ExtraInfo.  This allows us to
	// Inject our custom help text below
	cli.HelpPrinter = func(out io.Writer, templ string, cmd any) {
		cli.HelpPrinterCustom(out, templ, cmd, map[string]any{
			"ExtraInfo": cmd.(*cli.Command).ExtraInfo,
		})
	}

	var chdirArg string
	root := command.RootCommander(&chdirArg)

	// Prefix the args with any args from the EnvCLI
	subcommand := detectSubcommand(root)
	args, err := mergeEnvArgs(EnvCLI, subcommand, os.Args)
	if err != nil {
		rv.Error(err.Error())
		return 1
	}

	// Prefix the args with any args from the EnvCLI targeting this command
	suffix := strings.ReplaceAll(strings.ReplaceAll(
		subcommand, "-", "_"), " ", "_")
	args, err = mergeEnvArgs(
		fmt.Sprintf("%s_%s", EnvCLI, suffix), subcommand, args)
	if err != nil {
		rv.Error(err.Error())
		return 1
	}

	// In practice, this is only ever called once, though we could wrap it in a sync.Once if we want to be safer.
	meta := func() (command.Meta, int) {
		// Create the workdir and apply the -chdir options.
		// The logic inside [NewWorkdir] handles the TF_DATA_DIR env var too.
		wd, err := workdir.NewWorkdirExplicit(chdirArg)
		if err != nil {
			rv.Error(err.Error())
			return command.Meta{}, 1
		}

		providerSrc, diags := providerSource(ctx,
			config.ProviderInstallation,
			config.RegistryProtocols,
			services,
			config.OCICredentialsPolicy,
			wd.RootModuleDir(), // this has to be the directory that tofu has been executed from, not the one after -chdir
		)
		if len(diags) > 0 {
			rv.Error("There are some problems with the provider_installation configuration:")
			rv.Diagnostics(diags)
			if diags.HasErrors() {
				rv.Error("As a result of the above problems, OpenTofu's provider installer may not behave as intended.\n\n")
				// We continue to run anyway, because most commands don't do provider installation.
			}
		}

		// Attempt to ensure the config directory exists.
		configDir, err := cliconfig.ConfigDir()
		if err != nil {
			log.Printf("[ERROR] Failed to find the path to the config directory: %v", err)
		} else if err := mkConfigDir(configDir); err != nil {
			log.Printf("[ERROR] Failed to create the config directory at path %s: %v", configDir, err)
		}

		return makeMeta(ctx, wd, view, config, services, modulePkgFetcher, providerSrc, providerDevOverrides, unmanagedProviders), 0
	}

	rootCmd := commandToCli("", root, meta)

	rootCmd.EnableShellCompletion = true

	err = rootCmd.Run(context.Background(), args)
	if err != nil {
		if exitCode, ok := err.(ExitCodeError); ok {
			return int(exitCode)
		}
		rv.Error(fmt.Sprintf("Error executing CLI: %s", err.Error()))
		return 1
	}
	return 0
}

func commandToCli(namespace string, cmd command.Command, meta func() (command.Meta, int)) *cli.Command {
	cc := &cli.Command{
		Name:        cmd.Name,
		Description: cmd.Short,
	}

	subNs := namespace
	if cmd.Name != "" {
		subNs = namespace + cmd.Name + " "
	}

	for _, subCmd := range cmd.Commands {
		cc.Commands = append(cc.Commands, commandToCli(subNs, subCmd, meta))
	}

	// Setup custom help text
	cc.ExtraInfo = func() map[string]string {
		var usage strings.Builder
		command.CommandUsage(namespace, cmd, &usage)
		return map[string]string{"USAGE": usage.String()}
	}
	cc.CustomHelpTemplate = `{{ index ExtraInfo "USAGE" }}`

	if namespace == "" {
		cc.CustomRootCommandHelpTemplate = cc.CustomHelpTemplate
		cc.CommandNotFound = func(_ context.Context, c *cli.Command, given string) {
			suggestions := make([]string, 0, len(cmd.Commands))
			for _, sub := range cmd.Commands {
				suggestions = append(suggestions, sub.Name)
			}
			suggestion := didyoumean.NameSuggestion(given, suggestions)
			if suggestion != "" {
				suggestion = fmt.Sprintf(" Did you mean %q?", suggestion)
			}
			fmt.Fprintf(c.Writer, "OpenTofu has no command named %q.%s\n\nTo see all of OpenTofu's top-level commands, run:\n  tofu -help\n\n", given, suggestion)
			os.Exit(1)
		}
	}

	cc.Flags = cmd.CommandLine.CliFlags()
	cc.Arguments = cmd.CommandLine.CliArguments()

	// We are only using the above to determine the subcommand
	if meta == nil {
		return cc
	}

	if cmd.Run != nil {
		cc.Action = func(_ context.Context, _ *cli.Command) error {
			m, ec := meta()
			if ec != 0 {
				return ExitCodeError(ec)
			}

			// Check positional arguments
			diags := cmd.CommandLine.RemainCheck(cc.Args().Slice())

			ec = command.RunCli(namespace, cmd, m, diags)
			if ec != 0 {
				if ec == command.RunResultHelp {
					command.CommandUsage(namespace, cmd, os.Stdout)
					ec = 1
				}
				return ExitCodeError(ec)
			}
			return nil
		}
	}

	cc.OnUsageError = func(ctx context.Context, _ *cli.Command, err error, isSubcommand bool) error {
		m, ec := meta()
		if ec != 0 {
			return ExitCodeError(ec)
		}

		diags := tfdiags.New(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line options",
			err.Error(),
		))
		command.RunCli(namespace, cmd, m, diags)
		command.CommandUsage(namespace, cmd, os.Stdout)
		return ExitCodeError(1)
	}

	return cc
}

type ExitCodeError int

func (e ExitCodeError) Error() string {
	return fmt.Sprintf("%#v", e)
}

// detectSubCommand builds a temporary command line structure to
// determine what subcommand is actually being run. Ideally,
// we would be able to inject args as part of the above command
// runner, but that's not possible in the current urfave version.
func detectSubcommand(cmd command.Command) string {
	var subcommand string

	cc := commandToCli("", cmd, nil)
	cc.Walk(func(c *cli.Command) error {
		c.Action = func(context.Context, *cli.Command) error {
			subcommand = strings.Join(c.Path()[1:], " ")
			return nil
		}
		return nil
	})

	// Kill any potential output (especially if we are running shell complete)
	cc.Writer = ioutil.Discard
	cc.ErrWriter = ioutil.Discard

	// Ignore error code
	cc.Run(context.Background(), os.Args)

	return subcommand
}
