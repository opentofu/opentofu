package command

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	backendInit "github.com/opentofu/opentofu/internal/backend/init"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/providercache"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tracing"
	"github.com/opentofu/opentofu/internal/tracing/traceattrs"
	tfversion "github.com/opentofu/opentofu/version"
	"github.com/opentofu/svchost"
	"github.com/zclconf/go-cty/cty"
)

type initCfg struct {
	flagFromModule, flagLockfile, testsDirectory string
	flagBackend, flagCloud, flagGet, flagUpgrade bool
	flagPluginPath                               FlagStringSlice
	backendFlagSet                               bool
	cloudFlagSet                                 bool
	flagConfigExtra                              rawFlags
}

type initActs struct {
	*Meta
	*initCfg
}

func (c *initActs) initBackend(ctx context.Context, root *configs.Module, extraConfig rawFlags, enc encryption.Encryption) (be backend.Backend, output bool, diags tfdiags.Diagnostics) {
	ctx, span := tracing.Tracer().Start(ctx, "Backend init")
	_ = ctx // prevent staticcheck from complaining to avoid a maintenance hazard of having the wrong ctx in scope here
	defer span.End()

	c.Ui.Output(c.Colorize().Color("\n[reset][bold]Initializing the backend..."))

	var backendConfig *configs.Backend
	var backendConfigOverride hcl.Body
	if root.Backend != nil {
		backendType := root.Backend.Type
		if backendType == "cloud" {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unsupported backend type",
				Detail:   fmt.Sprintf("There is no explicit backend type named %q. To configure cloud backend, declare a 'cloud' block instead.", backendType),
				Subject:  &root.Backend.TypeRange,
			})
			return nil, true, diags
		}

		bf, canonType := backendInit.Backend(backendType)
		if bf == nil {
			detail := fmt.Sprintf("There is no backend type named %q.", backendType)
			if msg, removed := backendInit.RemovedBackends[backendType]; removed {
				detail = msg
			}

			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unsupported backend type",
				Detail:   detail,
				Subject:  &root.Backend.TypeRange,
			})
			return nil, true, diags
		}
		if backendType != canonType {
			c.Ui.Output(fmt.Sprintf("- %q is an alias for backend type %q", backendType, canonType))
		}

		b := bf(nil) // This is only used to get the schema, encryption should panic if attempted
		backendSchema := b.ConfigSchema()
		backendConfig = root.Backend

		var overrideDiags tfdiags.Diagnostics
		backendConfigOverride, overrideDiags = c.backendConfigOverrideBody(extraConfig, backendSchema)
		diags = diags.Append(overrideDiags)
		if overrideDiags.HasErrors() {
			return nil, true, diags
		}
	} else {
		// If the user supplied a -backend-config on the CLI but no backend
		// block was found in the configuration, it's likely - but not
		// necessarily - a mistake. Return a warning.
		if !extraConfig.Empty() {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Warning,
				"Missing backend configuration",
				`-backend-config was used without a "backend" block in the configuration.

If you intended to override the default local backend configuration,
no action is required, but you may add an explicit backend block to your
configuration to clear this warning:

terraform {
  backend "local" {}
}

However, if you intended to override a defined backend, please verify that
the backend configuration is present and valid.
`,
			))
		}
	}

	opts := &BackendOpts{
		Config:         backendConfig,
		ConfigOverride: backendConfigOverride,
		Init:           true,
	}

	back, backDiags := c.Backend(ctx, opts, enc.State())
	diags = diags.Append(backDiags)
	return back, true, diags
}

// backendConfigOverrideBody interprets the raw values of -backend-config
// arguments into a hcl Body that should override the backend settings given
// in the configuration.
//
// If the result is nil then no override needs to be provided.
//
// If the returned diagnostics contains errors then the returned body may be
// incomplete or invalid.
func (c *initActs) backendConfigOverrideBody(flags rawFlags, schema *configschema.Block) (hcl.Body, tfdiags.Diagnostics) {
	items := flags.AllItems()
	if len(items) == 0 {
		return nil, nil
	}

	var ret hcl.Body
	var diags tfdiags.Diagnostics
	synthVals := make(map[string]cty.Value)

	mergeBody := func(newBody hcl.Body) {
		if ret == nil {
			ret = newBody
		} else {
			ret = configs.MergeBodies(ret, newBody)
		}
	}
	flushVals := func() {
		if len(synthVals) == 0 {
			return
		}
		newBody := configs.SynthBody("-backend-config=...", synthVals)
		mergeBody(newBody)
		synthVals = make(map[string]cty.Value)
	}

	if len(items) == 1 && items[0].Value == "" {
		// Explicitly remove all -backend-config options.
		// We do this by setting an empty but non-nil ConfigOverrides.
		return configs.SynthBody("-backend-config=''", synthVals), diags
	}

	for _, item := range items {
		eq := strings.Index(item.Value, "=")

		if eq == -1 {
			// The value is interpreted as a filename.
			newBody, fileDiags := c.loadHCLFile(item.Value)
			diags = diags.Append(fileDiags)
			if fileDiags.HasErrors() {
				continue
			}
			// Generate an HCL body schema for the backend block.
			var bodySchema hcl.BodySchema
			for name := range schema.Attributes {
				// We intentionally ignore the `Required` attribute here
				// because backend config override files can be partial. The
				// goal is to make sure we're not loading a file with
				// extraneous attributes or blocks.
				bodySchema.Attributes = append(bodySchema.Attributes, hcl.AttributeSchema{
					Name: name,
				})
			}
			for name, block := range schema.BlockTypes {
				var labelNames []string
				if block.Nesting == configschema.NestingMap {
					labelNames = append(labelNames, "key")
				}
				bodySchema.Blocks = append(bodySchema.Blocks, hcl.BlockHeaderSchema{
					Type:       name,
					LabelNames: labelNames,
				})
			}
			// Verify that the file body matches the expected backend schema.
			_, schemaDiags := newBody.Content(&bodySchema)
			diags = diags.Append(schemaDiags)
			if schemaDiags.HasErrors() {
				continue
			}
			flushVals() // deal with any accumulated individual values first
			mergeBody(newBody)
		} else {
			name := item.Value[:eq]
			rawValue := item.Value[eq+1:]
			attrS := schema.Attributes[name]
			if attrS == nil {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Invalid backend configuration argument",
					fmt.Sprintf("The backend configuration argument %q given on the command line is not expected for the selected backend type.", name),
				))
				continue
			}
			value, valueDiags := configValueFromCLI(item.String(), rawValue, attrS.Type)
			diags = diags.Append(valueDiags)
			if valueDiags.HasErrors() {
				continue
			}
			synthVals[name] = value
		}
	}

	flushVals()

	return ret, diags
}

// Load the complete module tree, and fetch any missing providers.
// This method outputs its own Ui.
func (c *initActs) getProviders(ctx context.Context, config *configs.Config, state *states.State, upgrade bool, pluginDirs []string, flagLockfile string) (output, abort bool, diags tfdiags.Diagnostics) {
	ctx, span := tracing.Tracer().Start(ctx, "Get Providers")
	defer span.End()

	// Dev overrides cause the result of "tofu init" to be irrelevant for
	// any overridden providers, so we'll warn about it to avoid later
	// confusion when OpenTofu ends up using a different provider than the
	// lock file called for.
	diags = diags.Append(c.providerDevOverrideInitWarnings())

	// First we'll collect all the provider dependencies we can see in the
	// configuration and the state.
	reqs, qualifs, hclDiags := config.ProviderRequirements()
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return false, true, diags
	}
	if state != nil {
		stateReqs := state.ProviderRequirements()
		reqs = reqs.Merge(stateReqs)
	}

	potentialProviderConflicts := make(map[string][]string)

	for providerAddr := range reqs {
		if providerAddr.Namespace == "hashicorp" || providerAddr.Namespace == "opentofu" {
			potentialProviderConflicts[providerAddr.Type] = append(potentialProviderConflicts[providerAddr.Type], providerAddr.ForDisplay())
		}

		if providerAddr.IsLegacy() {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid legacy provider address",
				fmt.Sprintf(
					"This configuration or its associated state refers to the unqualified provider %q.\n\nYou must complete the Terraform 0.13 upgrade process before upgrading to later versions.",
					providerAddr.Type,
				),
			))
		}
	}

	for name, addrs := range potentialProviderConflicts {
		if len(addrs) > 1 {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Warning,
				"Potential provider misconfiguration",
				fmt.Sprintf(
					"OpenTofu has detected multiple providers of type %s (%s) which may be a misconfiguration.\n\nIf this is intentional you can ignore this warning",
					name,
					strings.Join(addrs, ", "),
				),
			))
		}
	}

	previousLocks, moreDiags := c.lockedDependenciesWithPredecessorRegistryShimmed()
	diags = diags.Append(moreDiags)

	if diags.HasErrors() {
		return false, true, diags
	}

	var inst *providercache.Installer
	if len(pluginDirs) == 0 {
		// By default we use a source that looks for providers in all of the
		// standard locations, possibly customized by the user in CLI config.
		inst = c.providerInstaller()
	} else {
		// If the user passes at least one -plugin-dir then that circumvents
		// the usual sources and forces OpenTofu to consult only the given
		// directories. Anything not available in one of those directories
		// is not available for installation.
		source := c.providerCustomLocalDirectorySource(ctx, pluginDirs)
		inst = c.providerInstallerCustomSource(source)

		// The default (or configured) search paths are logged earlier, in provider_source.go
		// Log that those are being overridden by the `-plugin-dir` command line options
		log.Println("[DEBUG] init: overriding provider plugin search paths")
		log.Printf("[DEBUG] will search for provider plugins in %s", pluginDirs)
	}

	// We want to print out a nice warning if we don't manage to pull
	// checksums for all our providers. This is tracked via callbacks
	// and incomplete providers are stored here for later analysis.
	var incompleteProviders []string

	// Because we're currently just streaming a series of events sequentially
	// into the terminal, we're showing only a subset of the events to keep
	// things relatively concise. Later it'd be nice to have a progress UI
	// where statuses update in-place, but we can't do that as long as we
	// are shimming our vt100 output to the legacy console API on Windows.
	evts := &providercache.InstallerEvents{
		PendingProviders: func(reqs map[addrs.Provider]getproviders.VersionConstraints) {
			c.Ui.Output(c.Colorize().Color(
				"\n[reset][bold]Initializing provider plugins...",
			))
		},
		ProviderAlreadyInstalled: func(provider addrs.Provider, selectedVersion getproviders.Version, inProviderCache bool) {
			if inProviderCache {
				c.Ui.Info(fmt.Sprintf("- Detected previously-installed %s v%s in the shared cache directory", provider.ForDisplay(), selectedVersion))
			} else {
				c.Ui.Info(fmt.Sprintf("- Using previously-installed %s v%s", provider.ForDisplay(), selectedVersion))
			}
		},
		BuiltInProviderAvailable: func(provider addrs.Provider) {
			c.Ui.Info(fmt.Sprintf("- %s is built in to OpenTofu", provider.ForDisplay()))
		},
		BuiltInProviderFailure: func(provider addrs.Provider, err error) {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid dependency on built-in provider",
				fmt.Sprintf("Cannot use %s: %s.", provider.ForDisplay(), err),
			))
		},
		QueryPackagesBegin: func(provider addrs.Provider, versionConstraints getproviders.VersionConstraints, locked bool) {
			if locked {
				c.Ui.Info(fmt.Sprintf("- Reusing previous version of %s from the dependency lock file", provider.ForDisplay()))
			} else {
				if len(versionConstraints) > 0 {
					c.Ui.Info(fmt.Sprintf("- Finding %s versions matching %q...", provider.ForDisplay(), getproviders.VersionConstraintsString(versionConstraints)))
				} else {
					c.Ui.Info(fmt.Sprintf("- Finding latest version of %s...", provider.ForDisplay()))
				}
			}
		},
		LinkFromCacheBegin: func(provider addrs.Provider, version getproviders.Version, cacheRoot string) {
			c.Ui.Info(fmt.Sprintf("- Using %s v%s from the shared cache directory", provider.ForDisplay(), version))
		},
		FetchPackageBegin: func(provider addrs.Provider, version getproviders.Version, location getproviders.PackageLocation, inProviderCache bool) {
			if inProviderCache {
				c.Ui.Info(fmt.Sprintf("- Installing %s v%s to the shared cache directory...", provider.ForDisplay(), version))
			} else {
				c.Ui.Info(fmt.Sprintf("- Installing %s v%s...", provider.ForDisplay(), version))
			}
		},
		QueryPackagesFailure: func(provider addrs.Provider, err error) {
			switch errorTy := err.(type) {
			case getproviders.ErrProviderNotFound:
				sources := errorTy.Sources
				displaySources := make([]string, len(sources))
				for i, source := range sources {
					displaySources[i] = fmt.Sprintf("  - %s", source)
				}
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Failed to query available provider packages",
					fmt.Sprintf("Could not retrieve the list of available versions for provider %s: %s\n\n%s",
						provider.ForDisplay(), err, strings.Join(displaySources, "\n"),
					),
				))
			case getproviders.ErrRegistryProviderNotKnown:
				// We might be able to suggest an alternative provider to use
				// instead of this one.
				suggestion := fmt.Sprintf("\n\nAll modules should specify their required_providers so that external consumers will get the correct providers when using a module. To see which modules are currently depending on %s, run the following command:\n    tofu providers", provider.ForDisplay())
				alternative := getproviders.MissingProviderSuggestion(ctx, provider, inst.ProviderSource(), reqs)
				if alternative != provider {
					suggestion = fmt.Sprintf(
						"\n\nDid you intend to use %s? If so, you must specify that source address in each module which requires that provider. To see which modules are currently depending on %s, run the following command:\n    tofu providers",
						alternative.ForDisplay(), provider.ForDisplay(),
					)
				}

				if provider.Hostname == addrs.DefaultProviderRegistryHost {
					suggestion += "\n\nIf you believe this provider is missing from the registry, please submit a issue on the OpenTofu Registry https://github.com/opentofu/registry/issues/new/choose"
				}

				warnDiags := warnOnFailedImplicitProvReference(provider, qualifs)
				diags = diags.Append(warnDiags)

				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Failed to query available provider packages",
					fmt.Sprintf("Could not retrieve the list of available versions for provider %s: %s%s",
						provider.ForDisplay(), err, suggestion,
					),
				))
			case getproviders.ErrHostNoProviders:
				switch {
				case errorTy.Hostname == svchost.Hostname("github.com") && !errorTy.HasOtherVersion:
					// If a user copies the URL of a GitHub repository into
					// the source argument and removes the schema to make it
					// provider-address-shaped then that's one way we can end up
					// here. We'll use a specialized error message in anticipation
					// of that mistake. We only do this if github.com isn't a
					// provider registry, to allow for the (admittedly currently
					// rather unlikely) possibility that github.com starts being
					// a real Terraform provider registry in the future.
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						"Invalid provider registry host",
						fmt.Sprintf("The given source address %q specifies a GitHub repository rather than a OpenTofu provider. Refer to the documentation of the provider to find the correct source address to use.",
							provider.String(),
						),
					))

				case errorTy.HasOtherVersion:
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						"Invalid provider registry host",
						fmt.Sprintf("The host %q given in provider source address %q does not offer a OpenTofu provider registry that is compatible with this OpenTofu version, but it may be compatible with a different OpenTofu version.",
							errorTy.Hostname, provider.String(),
						),
					))

				default:
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						"Invalid provider registry host",
						fmt.Sprintf("The host %q given in provider source address %q does not offer a OpenTofu provider registry.",
							errorTy.Hostname, provider.String(),
						),
					))
				}

			case getproviders.ErrRequestCanceled:
				// We don't attribute cancellation to any particular operation,
				// but rather just emit a single general message about it at
				// the end, by checking ctx.Err().

			default:
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Failed to resolve provider packages",
					fmt.Sprintf("Could not resolve provider %s: %s",
						provider.ForDisplay(), err,
					),
				))
			}

		},
		QueryPackagesWarning: func(provider addrs.Provider, warnings []string) {
			displayWarnings := make([]string, len(warnings))
			for i, warning := range warnings {
				displayWarnings[i] = fmt.Sprintf("- %s", warning)
			}

			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Warning,
				"Additional provider information from registry",
				fmt.Sprintf("The remote registry returned warnings for %s:\n%s",
					provider.String(),
					strings.Join(displayWarnings, "\n"),
				),
			))
		},
		LinkFromCacheFailure: func(provider addrs.Provider, version getproviders.Version, err error) {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Failed to install provider from shared cache",
				fmt.Sprintf("Error while importing %s v%s from the shared cache directory: %s.", provider.ForDisplay(), version, err),
			))
		},
		FetchPackageFailure: func(provider addrs.Provider, version getproviders.Version, err error) {
			const summaryIncompatible = "Incompatible provider version"
			switch err := err.(type) {
			case getproviders.ErrProtocolNotSupported:
				closestAvailable := err.Suggestion
				switch {
				case closestAvailable == getproviders.UnspecifiedVersion:
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						summaryIncompatible,
						fmt.Sprintf(errProviderVersionIncompatible, provider.String()),
					))
				case version.GreaterThan(closestAvailable):
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						summaryIncompatible,
						fmt.Sprintf(providerProtocolTooNew, provider.ForDisplay(),
							version, tfversion.String(), closestAvailable, closestAvailable,
							getproviders.VersionConstraintsString(reqs[provider]),
						),
					))
				default: // version is less than closestAvailable
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						summaryIncompatible,
						fmt.Sprintf(providerProtocolTooOld, provider.ForDisplay(),
							version, tfversion.String(), closestAvailable, closestAvailable,
							getproviders.VersionConstraintsString(reqs[provider]),
						),
					))
				}
			case getproviders.ErrPlatformNotSupported:
				switch {
				case err.MirrorURL != nil:
					// If we're installing from a mirror then it may just be
					// the mirror lacking the package, rather than it being
					// unavailable from upstream.
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						summaryIncompatible,
						fmt.Sprintf(
							"Your chosen provider mirror at %s does not have a %s v%s package available for your current platform, %s.\n\nProvider releases are separate from OpenTofu CLI releases, so this provider might not support your current platform. Alternatively, the mirror itself might have only a subset of the plugin packages available in the origin registry, at %s.",
							err.MirrorURL, err.Provider, err.Version, err.Platform,
							err.Provider.Hostname,
						),
					))
				default:
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						summaryIncompatible,
						fmt.Sprintf(
							"Provider %s v%s does not have a package available for your current platform, %s.\n\nProvider releases are separate from OpenTofu CLI releases, so not all providers are available for all platforms. Other versions of this provider may have different platforms supported.",
							err.Provider, err.Version, err.Platform,
						),
					))
				}

			case getproviders.ErrRequestCanceled:
				// We don't attribute cancellation to any particular operation,
				// but rather just emit a single general message about it at
				// the end, by checking ctx.Err().

			default:
				// We can potentially end up in here under cancellation too,
				// in spite of our getproviders.ErrRequestCanceled case above,
				// because not all of the outgoing requests we do under the
				// "fetch package" banner are source metadata requests.
				// In that case we will emit a redundant error here about
				// the request being cancelled, but we'll still detect it
				// as a cancellation after the installer returns and do the
				// normal cancellation handling.

				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Failed to install provider",
					fmt.Sprintf("Error while installing %s v%s: %s", provider.ForDisplay(), version, err),
				))
			}
		},
		FetchPackageSuccess: func(provider addrs.Provider, version getproviders.Version, localDir string, authResult *getproviders.PackageAuthenticationResult) {
			var keyID string
			if authResult != nil && authResult.Signed() {
				keyID = authResult.GPGKeyIDsString()
			}
			if keyID != "" {
				keyID = c.Colorize().Color(fmt.Sprintf(", key ID [reset][bold]%s[reset]", keyID))
			}

			if authResult != nil && authResult.SigningSkipped() {
				c.Ui.Warn(fmt.Sprintf("- Installed %s v%s. Signature validation was skipped due to the registry not containing GPG keys for this provider", provider.ForDisplay(), version))
			} else {
				c.Ui.Info(fmt.Sprintf("- Installed %s v%s (%s%s)", provider.ForDisplay(), version, authResult, keyID))
			}
		},
		CacheDirLockContended: func(cacheDir string) {
			c.Ui.Info(fmt.Sprintf("- Waiting for lock on cache directory %s", cacheDir))
		},
		ProvidersLockUpdated: func(provider addrs.Provider, version getproviders.Version, localHashes []getproviders.Hash, signedHashes []getproviders.Hash, priorHashes []getproviders.Hash) {
			// We're going to use this opportunity to track if we have any
			// "incomplete" installs of providers. An incomplete install is
			// when we are only going to write the local hashes into our lock
			// file which means a `tofu init` command will fail in future
			// when used on machines of a different architecture.
			//
			// We want to print a warning about this.

			if len(signedHashes) > 0 {
				// If we have any signedHashes hashes then we don't worry - as
				// we know we retrieved all available hashes for this version
				// anyway.
				return
			}

			// If local hashes and prior hashes are exactly the same then
			// it means we didn't record any signed hashes previously, and
			// we know we're not adding any extra in now (because we already
			// checked the signedHashes), so that's a problem.
			//
			// In the actual check here, if we have any priorHashes and those
			// hashes are not the same as the local hashes then we're going to
			// accept that this provider has been configured correctly.
			if len(priorHashes) > 0 && !reflect.DeepEqual(localHashes, priorHashes) {
				return
			}

			// Now, either signedHashes is empty, or priorHashes is exactly the
			// same as our localHashes which means we never retrieved the
			// signedHashes previously.
			//
			// Either way, this is bad. Let's complain/warn.
			incompleteProviders = append(incompleteProviders, provider.ForDisplay())
		},
		ProvidersAuthenticated: func(authResults map[addrs.Provider]*getproviders.PackageAuthenticationResult) {
			thirdPartySigned := false
			for _, authResult := range authResults {
				if authResult.Signed() {
					thirdPartySigned = true
					break
				}
			}
			if thirdPartySigned {
				c.Ui.Info(fmt.Sprintf("\nProviders are signed by their developers.\n" +
					"If you'd like to know more about provider signing, you can read about it here:\n" +
					"https://opentofu.org/docs/cli/plugins/signing/"))
			}
		},
	}
	ctx = evts.OnContext(ctx)

	mode := providercache.InstallNewProvidersOnly
	if upgrade {
		if flagLockfile == "readonly" {
			c.Ui.Error("The -upgrade flag conflicts with -lockfile=readonly.")
			return true, true, diags
		}

		mode = providercache.InstallUpgrades
	}
	newLocks, err := inst.EnsureProviderVersions(ctx, previousLocks, reqs, mode)
	if ctx.Err() == context.Canceled {
		c.showDiagnostics(diags)
		c.Ui.Error("Provider installation was canceled by an interrupt signal.")
		return true, true, diags
	}
	if err != nil {
		// The errors captured in "err" should be redundant with what we
		// received via the InstallerEvents callbacks above, so we'll
		// just return those as long as we have some.
		if !diags.HasErrors() {
			diags = diags.Append(err)
		}

		return true, true, diags
	}

	// If the provider dependencies have changed since the last run then we'll
	// say a little about that in case the reader wasn't expecting a change.
	// (When we later integrate module dependencies into the lock file we'll
	// probably want to refactor this so that we produce one lock-file related
	// message for all changes together, but this is here for now just because
	// it's the smallest change relative to what came before it, which was
	// a hidden JSON file specifically for tracking providers.)
	if !newLocks.Equal(previousLocks) {
		// if readonly mode
		if flagLockfile == "readonly" {
			// check if required provider dependencies change
			if !newLocks.EqualProviderAddress(previousLocks) {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					`Provider dependency changes detected`,
					`Changes to the required provider dependencies were detected, but the lock file is read-only. To use and record these requirements, run "tofu init" without the "-lockfile=readonly" flag.`,
				))
				return true, true, diags
			}

			// suppress updating the file to record any new information it learned,
			// such as a hash using a new scheme.
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Warning,
				`Provider lock file not updated`,
				`Changes to the provider selections were detected, but not saved in the .terraform.lock.hcl file. To record these selections, run "tofu init" without the "-lockfile=readonly" flag.`,
			))
			return true, false, diags
		}

		// Jump in here and add a warning if any of the providers are incomplete.
		if len(incompleteProviders) > 0 {
			// We don't really care about the order here, we just want the
			// output to be deterministic.
			sort.Slice(incompleteProviders, func(i, j int) bool {
				return incompleteProviders[i] < incompleteProviders[j]
			})
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Warning,
				incompleteLockFileInformationHeader,
				fmt.Sprintf(
					incompleteLockFileInformationBody,
					strings.Join(incompleteProviders, "\n  - "),
					getproviders.CurrentPlatform.String())))
		}

		if previousLocks.Empty() {
			// A change from empty to non-empty is special because it suggests
			// we're running "tofu init" for the first time against a
			// new configuration. In that case we'll take the opportunity to
			// say a little about what the dependency lock file is, for new
			// users or those who are upgrading from a previous Terraform
			// version that didn't have dependency lock files.
			c.Ui.Output(c.Colorize().Color(`
OpenTofu has created a lock file [bold].terraform.lock.hcl[reset] to record the provider
selections it made above. Include this file in your version control repository
so that OpenTofu can guarantee to make the same selections by default when
you run "tofu init" in the future.`))
		} else {
			c.Ui.Output(c.Colorize().Color(`
OpenTofu has made some changes to the provider dependency selections recorded
in the .terraform.lock.hcl file. Review those changes and commit them to your
version control system if they represent changes you intended to make.`))
		}

		moreDiags = c.replaceLockedDependencies(ctx, newLocks)
		diags = diags.Append(moreDiags)
	}

	return true, false, diags
}

func (c *initActs) initCloud(ctx context.Context, root *configs.Module, extraConfig rawFlags, enc encryption.Encryption) (be backend.Backend, output bool, diags tfdiags.Diagnostics) {
	ctx, span := tracing.Tracer().Start(ctx, "Cloud backend init")
	_ = ctx // prevent staticcheck from complaining to avoid a maintenance hazard of having the wrong ctx in scope here
	defer span.End()

	c.Ui.Output(c.Colorize().Color("\n[reset][bold]Initializing cloud backend..."))

	if len(extraConfig.AllItems()) != 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid command-line option",
			"The -backend-config=... command line option is only for state backends, and is not applicable to cloud backend-based configurations.\n\nTo change the set of workspaces associated with this configuration, edit the Cloud configuration block in the root module.",
		))
		return nil, true, diags
	}

	backendConfig := root.CloudConfig.ToBackendConfig()

	opts := &BackendOpts{
		Config: &backendConfig,
		Init:   true,
	}

	back, backDiags := c.Backend(ctx, opts, enc.State())
	diags = diags.Append(backDiags)
	return back, true, diags
}

func (c *initActs) getModules(ctx context.Context, path, testsDir string, earlyRoot *configs.Module, upgrade bool) (output bool, abort bool, diags tfdiags.Diagnostics) {
	testModules := false // We can also have modules buried in test files.
	for _, file := range earlyRoot.Tests {
		for _, run := range file.Runs {
			if run.Module != nil {
				testModules = true
			}
		}
	}

	if len(earlyRoot.ModuleCalls) == 0 && !testModules {
		// Nothing to do
		return false, false, nil
	}

	ctx, span := tracing.Tracer().Start(ctx, "Get Modules", tracing.SpanAttributes(
		traceattrs.Bool("opentofu.modules.upgrade", upgrade),
	))
	defer span.End()

	if upgrade {
		c.Ui.Output(c.Colorize().Color("[reset][bold]Upgrading modules..."))
	} else {
		c.Ui.Output(c.Colorize().Color("[reset][bold]Initializing modules..."))
	}

	hooks := uiModuleInstallHooks{
		Ui:             c.Ui,
		ShowLocalPaths: true,
	}

	installAbort, installDiags := c.installModules(ctx, path, testsDir, upgrade, false, hooks)
	diags = diags.Append(installDiags)

	// At this point, installModules may have generated error diags or been
	// aborted by SIGINT. In any case we continue and the manifest as best
	// we can.

	// Since module installer has modified the module manifest on disk, we need
	// to refresh the cache of it in the loader.
	if c.configLoader != nil {
		if err := c.configLoader.RefreshModules(); err != nil {
			// Should never happen
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Failed to read module manifest",
				fmt.Sprintf("After installing modules, OpenTofu could not re-read the manifest of installed modules. This is a bug in OpenTofu. %s.", err),
			))
		}
	}

	return true, installAbort, diags
}
