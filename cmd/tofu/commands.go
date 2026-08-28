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
	"github.com/hashicorp/go-retryablehttp"
	"github.com/opentofu/opentofu/internal/command/system"
	"github.com/opentofu/opentofu/internal/command/workdir"
	"github.com/opentofu/svchost"
	"github.com/opentofu/svchost/disco"
	"github.com/opentofu/svchost/svcauth"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command"
	"github.com/opentofu/opentofu/internal/command/cliconfig"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/getmodules"
	"github.com/opentofu/opentofu/internal/getproviders"
	pluginDiscovery "github.com/opentofu/opentofu/internal/plugin/discovery"
)

// runningInAutomationEnvName gives the name of an environment variable that
// can be set to any non-empty value in order to suppress certain messages
// that assume that OpenTofu is being run from a command prompt.
const runningInAutomationEnvName = "TF_IN_AUTOMATION"

func makeMeta(
	ctx context.Context,
	wd *workdir.Dir,
	view *views.View,
	config *cliconfig.Config,
	services *disco.Disco,
	modulePkgFetcher *getmodules.PackageFetcher,
	providerSrc getproviders.Source,
	providerDevOverrides map[addrs.Provider]getproviders.PackageLocalDir,
	unmanagedProviders map[addrs.Provider]*plugin.ReattachConfig,
) command.Meta {
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

	return command.Meta{
		WorkingDir: wd,
		View:       view.SetRunningInAutomation(inAutomation),
		SystemCfg: system.Config{
			RunningInAutomation:       inAutomation,
			CLIConfigDir:              configDir,
			PluginCacheDir:            config.PluginCacheDir,
			GlobalPluginDirs:          globalPluginDirs(),
			E2ETestingFeaturesEnabled: e2eTestingFeaturesEnabled(),
		},

		Services:        services,
		BrowserLauncher: browserLauncher(),

		PluginCacheMayBreakDependencyLockFile: config.PluginCacheMayBreakDependencyLockFile,

		ShutdownCh:    makeShutdownCh(),
		CallerContext: ctx,

		MakeRegistryHTTPClient: func() *retryablehttp.Client {
			// This ctx is used only to choose global configuration settings
			// for the client, and is not retained as part of the result for
			// making individual HTTP requests.
			return newRegistryHTTPClient(ctx, config.RegistryProtocols)
		},
		ModulePackageFetcher: modulePkgFetcher,
		ProviderSource:       providerSrc,
		ProviderDevOverrides: providerDevOverrides,
		UnmanagedProviders:   unmanagedProviders,

		// OCICredentialsPolicyBuilder is passed here for some commands (e.g. providers lock) that cannot
		// use ProvidersSource but still might need OCICredentials provided by the config
		OCICredentialsPolicyBuilder: config.OCICredentialsPolicy,

		// ProviderSourceLocationConfig is used for some commands that do not make
		// use of the OpenTofu configuration files. Therefore, there is no way to configure
		// the retries from other places than env vars.
		ProviderSourceLocationConfig: providerSourceLocationConfigFromEnv(),
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

func credentialsSource(config *cliconfig.Config) (svcauth.CredentialsSource, error) {
	helperPlugins := pluginDiscovery.FindPlugins("credentials", globalPluginDirs())
	return config.CredentialsSource(helperPlugins)
}
