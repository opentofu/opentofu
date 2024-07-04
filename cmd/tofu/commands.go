// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/hashicorp/go-plugin"
	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/hashicorp/terraform-svchost/auth"
	"github.com/hashicorp/terraform-svchost/disco"
	"github.com/mitchellh/cli"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command"
	"github.com/opentofu/opentofu/internal/command/cliconfig"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/command/webbrowser"
	"github.com/opentofu/opentofu/internal/getproviders"
	pluginDiscovery "github.com/opentofu/opentofu/internal/plugin/discovery"
	"github.com/opentofu/opentofu/internal/terminal"
)

// runningInAutomationEnvName gives the name of an environment variable that
// can be set to any non-empty value in order to suppress certain messages
// that assume that OpenTofu is being run from a command prompt.
const runningInAutomationEnvName = "TF_IN_AUTOMATION"

// commands is the mapping of all the available OpenTofu commands.
var commands map[string]cli.CommandFactory

// primaryCommands is an ordered sequence of the top-level commands (not
// subcommands) that we emphasize at the top of our help output. This is
// ordered so that we can show them in the typical workflow order, rather
// than in alphabetical order. Anything not in this sequence or in the
// HiddenCommands set appears under "all other commands".
var primaryCommands []string

// hiddenCommands is a set of top-level commands (not subcommands) that are
// not advertised in the top-level help at all. This is typically because
// they are either just stubs that return an error message about something
// no longer being supported or backward-compatibility aliases for other
// commands.
//
// No commands in the PrimaryCommands sequence should also appear in the
// hiddenCommands set, because that would be rather silly.
var hiddenCommands map[string]struct{}

// Ui is the cli.Ui used for communicating to the outside world.
var Ui cli.Ui

// Command is the interface required to wrap a command and check if a pedantic warning has been flagged
type Command interface {
	cli.Command
	WarningFlagged() bool
}

// Wraps a command
type commandWrapper struct {
	Command
}

// Run runs the command and returns the appropriate return code based on whether a
// pedantic warning has been flagged or not
func (cw *commandWrapper) Run(args []string) int {
	retCode := cw.Command.Run(args)
	if cw.Command.WarningFlagged() {
		retCode = 1
	}
	return retCode
}

func initCommands(
	ctx context.Context,
	originalWorkingDir string,
	streams *terminal.Streams,
	config *cliconfig.Config,
	services *disco.Disco,
	providerSrc getproviders.Source,
	providerDevOverrides map[addrs.Provider]getproviders.PackageLocalDir,
	unmanagedProviders map[addrs.Provider]*plugin.ReattachConfig,
	pedanticMode bool,
) {
	var inAutomation bool
	if v := os.Getenv(runningInAutomationEnvName); v != "" {
		inAutomation = true
	}

	for userHost, hostConfig := range config.Hosts {
		host, err := svchost.ForComparison(userHost)
		if err != nil {
			// We expect the config was already validated by the time we get
			// here, so we'll just ignore invalid hostnames.
			continue
		}
		services.ForceHostServices(host, hostConfig.Services)
	}

	configDir, err := cliconfig.ConfigDir()
	if err != nil {
		configDir = "" // No config dir available (e.g. looking up a home directory failed)
	}

	wd := workingDir(originalWorkingDir, os.Getenv("TF_DATA_DIR"))

	meta := command.Meta{
		WorkingDir: wd,
		Streams:    streams,
		View:       views.NewView(streams).SetRunningInAutomation(inAutomation),

		Color:            true,
		GlobalPluginDirs: globalPluginDirs(),
		Ui:               Ui,

		Services:        services,
		BrowserLauncher: webbrowser.NewNativeLauncher(),

		RunningInAutomation: inAutomation,
		CLIConfigDir:        configDir,
		PluginCacheDir:      config.PluginCacheDir,

		PluginCacheMayBreakDependencyLockFile: config.PluginCacheMayBreakDependencyLockFile,

		ShutdownCh:    makeShutdownCh(),
		CallerContext: ctx,

		ProviderSource:       providerSrc,
		ProviderDevOverrides: providerDevOverrides,
		UnmanagedProviders:   unmanagedProviders,

		AllowExperimentalFeatures: experimentsAreAllowed(),

		PedanticMode: pedanticMode,
	}

	// The command list is included in the tofu -help
	// output, which is in turn included in the docs at
	// website/docs/cli/commands/index.html.markdown; if you
	// add, remove or reclassify commands then consider updating
	// that to match.

	commands = map[string]cli.CommandFactory{
		"apply": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.ApplyCommand{
					Meta: meta,
				},
			}, nil
		},

		"console": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.ConsoleCommand{
					Meta: meta,
				},
			}, nil
		},

		"destroy": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.ApplyCommand{
					Meta:    meta,
					Destroy: true,
				},
			}, nil
		},

		"env": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.WorkspaceCommand{
					Meta:       meta,
					LegacyName: true,
				},
			}, nil
		},

		"env list": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.WorkspaceListCommand{
					Meta: meta,
					LegacyName: true,
				},
			}, nil
		},

		"env select": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.WorkspaceSelectCommand{
					Meta: meta,
					LegacyName: true,
				},
			}, nil
		},

		"env new": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.WorkspaceNewCommand{
					Meta:       meta,
					LegacyName: true,
				},
			}, nil
		},

		"env delete": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.WorkspaceDeleteCommand{
					Meta:       meta,
					LegacyName: true,
				},
			}, nil
		},

		"fmt": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.FmtCommand{
					Meta: meta,
				},
			}, nil
		},

		"get": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.GetCommand{
					Meta: meta,
				},
			}, nil
		},

		"graph": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.GraphCommand{
					Meta: meta,
				},
			}, nil
		},

		"import": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.ImportCommand{
					Meta: meta,
				},
			}, nil
		},

		"init": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.InitCommand{
					Meta: meta,
				},
			}, nil
		},

		"login": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.LoginCommand{
					Meta: meta,
				},
			}, nil
		},

		"logout": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.LogoutCommand{
					Meta: meta,
				},
			}, nil
		},

		"metadata": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.MetadataCommand{
					Meta: meta,
				},
			}, nil
		},

		"metadata functions": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.MetadataFunctionsCommand{
					Meta: meta,
				},
			}, nil
		},

		"output": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.OutputCommand{
					Meta: meta,
				},
			}, nil
		},

		"plan": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.PlanCommand{
					Meta: meta,
				},
			}, nil
		},

		"providers": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.ProvidersCommand{
					Meta: meta,
				},
			}, nil
		},

		"providers lock": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.ProvidersLockCommand{
					Meta: meta,
				},
			}, nil
		},

		"providers mirror": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.ProvidersMirrorCommand{
					Meta: meta,
				},
			}, nil
		},

		"providers schema": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.ProvidersSchemaCommand{
					Meta: meta,
				},
			}, nil
		},

		"push": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.PushCommand{
					Meta: meta,
				},
			}, nil
		},

		"refresh": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.RefreshCommand{
					Meta: meta,
				},
			}, nil
		},

		"show": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.ShowCommand{
					Meta: meta,
				},
			}, nil
		},

		"taint": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.TaintCommand{
					Meta: meta,
				},
			}, nil
		},

		"test": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.TestCommand{
					Meta: meta,
				},
			}, nil
		},

		"validate": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.ValidateCommand{
					Meta: meta,
				},
			}, nil
		},

		"version": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.VersionCommand{
					Meta:              meta,
					Version:           Version,
					VersionPrerelease: VersionPrerelease,
					Platform:          getproviders.CurrentPlatform,
				},
			}, nil
		},

		"untaint": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.UntaintCommand{
					Meta: meta,
				},
			}, nil
		},

		"workspace": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.WorkspaceCommand{
					Meta: meta,
				},
			}, nil
		},

		"workspace list": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.WorkspaceListCommand{
					Meta: meta,
				},
			}, nil
		},

		"workspace select": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.WorkspaceSelectCommand{
					Meta: meta,
				},
			}, nil
		},

		"workspace show": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.WorkspaceShowCommand{
					Meta: meta,
				},
			}, nil
		},

		"workspace new": func() (cli.Command, error) {
			return &commandWrapper{
				&command.WorkspaceNewCommand{
					Meta: meta,
				},
			}, nil
		},

		"workspace delete": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.WorkspaceDeleteCommand{
					Meta: meta,
				},
			}, nil
		},

		//-----------------------------------------------------------
		// Plumbing
		//-----------------------------------------------------------

		"force-unlock": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.UnlockCommand{
					Meta: meta,
				},
			}, nil
		},

		"state": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.StateCommand{
					StateMeta: command.StateMeta{
						Meta: meta,
					},
				},
			}, nil
		},

		"state list": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.StateListCommand{
					Meta: meta,
				},
			}, nil
		},

		"state ls": func() (cli.Command, error) {
			return &command.AliasCommand{
				Command: &commandWrapper{
					Command: &command.StateListCommand{
						Meta: meta,
					},
				},
			}, nil
		},

		"state rm": func() (cli.Command, error) {
			return &command.StateRmCommand{
				StateMeta: command.StateMeta{
					Meta: meta,
				},
			}, nil
		},

		"state remove": func() (cli.Command, error) {
			return &command.AliasCommand{
				Command: &commandWrapper{
					Command: &command.StateRmCommand{
						StateMeta: command.StateMeta{
							Meta: meta,
						},
					},
				},
			}, nil
		},

		"state mv": func() (cli.Command, error) {
			return &command.StateMvCommand{
				StateMeta: command.StateMeta{
					Meta: meta,
				},
			}, nil
		},

		"state move": func() (cli.Command, error) {
			return &command.AliasCommand{
				Command: &commandWrapper{
					Command: &command.StateMvCommand{
						StateMeta: command.StateMeta{
							Meta: meta,
						},
					},
				},
			}, nil
		},

		"state pull": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.StatePullCommand{
					Meta: meta,
				},
			}, nil
		},

		"state push": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.StatePushCommand{
					Meta: meta,
				},
			}, nil
		},

		"state show": func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.StateShowCommand{
					Meta: meta,
				},
			}, nil
		},

		"state replace-provider": func() (cli.Command, error) {
			return &command.StateReplaceProviderCommand{
				StateMeta: command.StateMeta{
					Meta: meta,
				},
			}, nil
		},
	}

	if meta.AllowExperimentalFeatures {
		commands["cloud"] = func() (cli.Command, error) {
			return &commandWrapper{
				Command: &command.CloudCommand{
					Meta: meta,
				},
			}, nil
		}
	}

	primaryCommands = []string{
		"init",
		"validate",
		"plan",
		"apply",
		"destroy",
	}

	hiddenCommands = map[string]struct{}{
		"env":             {},
		"internal-plugin": {},
		"push":            {},
	}
}

// makeShutdownCh creates an interrupt listener and returns a channel.
// A message will be sent on the channel for every interrupt received.
func makeShutdownCh() <-chan struct{} {
	resultCh := make(chan struct{})

	signalCh := make(chan os.Signal, 4)
	signal.Notify(signalCh, ignoreSignals...)
	signal.Notify(signalCh, forwardSignals...)
	go func() {
		for {
			<-signalCh
			resultCh <- struct{}{}
		}
	}()

	return resultCh
}

func credentialsSource(config *cliconfig.Config) (auth.CredentialsSource, error) {
	helperPlugins := pluginDiscovery.FindPlugins("credentials", globalPluginDirs())
	return config.CredentialsSource(helperPlugins)
}

func getAliasCommandKeys() []string {
	keys := []string{}
	for key, cmdFact := range commands {
		cmd, _ := cmdFact()
		_, ok := cmd.(*command.AliasCommand)
		if ok {
			keys = append(keys, key)
		}
	}
	return keys
}
