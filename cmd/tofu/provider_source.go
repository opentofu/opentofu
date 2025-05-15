// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"

	"github.com/apparentlymart/go-userdirs/userdirs"
	"github.com/opentofu/svchost/disco"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/cliconfig"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// providerSource constructs a provider source based on a combination of the
// CLI configuration and some default search locations. This will be the
// provider source used for provider installation in the "tofu init"
// command, unless overridden by the special -plugin-dir option.
func providerSource(configs []*cliconfig.ProviderInstallation, services *disco.Disco, getOCICredsPolicy ociCredsPolicyBuilder) (getproviders.Source, tfdiags.Diagnostics) {
	if len(configs) == 0 {
		// If there's no explicit installation configuration then we'll build
		// up an implicit one with direct registry installation along with
		// some automatically-selected local filesystem mirrors.
		return implicitProviderSource(services), nil
	}

	// There should only be zero or one configurations, which is checked by
	// the validation logic in the cliconfig package. Therefore we'll just
	// ignore any additional configurations in here.
	config := configs[0]
	return explicitProviderSource(config, services, getOCICredsPolicy)
}

func explicitProviderSource(config *cliconfig.ProviderInstallation, services *disco.Disco, getOCICredsPolicy ociCredsPolicyBuilder) (getproviders.Source, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var searchRules []getproviders.MultiSourceSelector

	log.Printf("[DEBUG] Explicit provider installation configuration is set")
	for _, methodConfig := range config.Methods {
		source, moreDiags := providerSourceForCLIConfigLocation(methodConfig.Location, services, getOCICredsPolicy)
		diags = diags.Append(moreDiags)
		if moreDiags.HasErrors() {
			continue
		}

		include, err := getproviders.ParseMultiSourceMatchingPatterns(methodConfig.Include)
		if err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid provider source inclusion patterns",
				fmt.Sprintf("CLI config specifies invalid provider inclusion patterns: %s.", err),
			))
			continue
		}
		exclude, err := getproviders.ParseMultiSourceMatchingPatterns(methodConfig.Exclude)
		if err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid provider source exclusion patterns",
				fmt.Sprintf("CLI config specifies invalid provider exclusion patterns: %s.", err),
			))
			continue
		}

		searchRules = append(searchRules, getproviders.MultiSourceSelector{
			Source:  source,
			Include: include,
			Exclude: exclude,
		})

		log.Printf("[TRACE] Selected provider installation method %#v with includes %s and excludes %s", methodConfig.Location, include, exclude)
	}

	return getproviders.MultiSource(searchRules), diags
}

// implicitProviderSource builds a default provider source to use if there's
// no explicit provider installation configuration in the CLI config.
//
// This implicit source looks in a number of local filesystem directories and
// directly in a provider's upstream registry. Any providers that have at least
// one version available in a local directory are implicitly excluded from
// direct installation, as if the user had listed them explicitly in the
// "exclude" argument in the direct provider source in the CLI config.
func implicitProviderSource(services *disco.Disco) getproviders.Source {
	// The local search directories we use for implicit configuration are:
	// - The "terraform.d/plugins" directory in the current working directory,
	//   which we've historically documented as a place to put plugins as a
	//   way to include them in bundles uploaded to Terraform Cloud, where
	//   there has historically otherwise been no way to use custom providers.
	// - The "plugins" subdirectory of the CLI config search directory.
	//   (that's ~/.terraform.d/plugins or $XDG_DATA_HOME/opentofu/plugins
	//   on Unix systems, equivalents elsewhere)
	// - The "plugins" subdirectory of any platform-specific search paths,
	//   following e.g. the XDG base directory specification on Unix systems,
	//   Apple's guidelines on OS X, and "known folders" on Windows.
	//
	// Any provider we find in one of those implicit directories will be
	// automatically excluded from direct installation from an upstream
	// registry. Anything not available locally will query its primary
	// upstream registry.
	var searchRules []getproviders.MultiSourceSelector

	// We'll track any providers we can find in the local search directories
	// along the way, and then exclude them from the registry source we'll
	// finally add at the end.
	foundLocally := map[addrs.Provider]struct{}{}

	addLocalDir := func(dir string) {
		// We'll make sure the directory actually exists before we add it,
		// because otherwise installation would always fail trying to look
		// in non-existent directories. (This is done here rather than in
		// the source itself because explicitly-selected directories via the
		// CLI config, once we have them, _should_ produce an error if they
		// don't exist to help users get their configurations right.)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			log.Printf("[DEBUG] will search for provider plugins in %s", dir)
			fsSource := getproviders.NewFilesystemMirrorSource(dir)

			// We'll peep into the source to find out what providers it seems
			// to be providing, so that we can exclude those from direct
			// install. This might fail, in which case we'll just silently
			// ignore it and assume it would fail during installation later too
			// and therefore effectively doesn't provide _any_ packages.
			if available, err := fsSource.AllAvailablePackages(); err == nil {
				for found := range available {
					foundLocally[found] = struct{}{}
				}
			}

			searchRules = append(searchRules, getproviders.MultiSourceSelector{
				Source: fsSource,
			})

		} else {
			log.Printf("[DEBUG] ignoring non-existing provider search directory %s", dir)
		}
	}

	addLocalDir("terraform.d/plugins") // our "vendor" directory
	cliDataDirs, err := cliconfig.DataDirs()
	if err == nil {
		for _, cliDataDir := range cliDataDirs {
			addLocalDir(filepath.Join(cliDataDir, "plugins"))
		}
	}

	// This "userdirs" library implements an appropriate user-specific and
	// app-specific directory layout for the current platform, such as XDG Base
	// Directory on Unix, using the following name strings to construct a
	// suitable application-specific subdirectory name following the
	// conventions for each platform:
	//
	//   XDG (Unix): lowercase of the first string, "terraform"
	//   Windows:    two-level hierarchy of first two strings, "HashiCorp\Terraform"
	//   OS X:       reverse-DNS unique identifier, "io.terraform".
	sysSpecificDirs := userdirs.ForApp("Terraform", "HashiCorp", "io.terraform")
	for _, dir := range sysSpecificDirs.DataSearchPaths("plugins") {
		addLocalDir(dir)
	}

	// Anything we found in local directories above is excluded from being
	// looked up via the registry source we're about to construct.
	var directExcluded getproviders.MultiSourceMatchingPatterns
	for addr := range foundLocally {
		directExcluded = append(directExcluded, addr)
	}

	// Last but not least, the main registry source! We'll wrap a caching
	// layer around this one to help optimize the several network requests
	// we'll end up making to it while treating it as one of several sources
	// in a MultiSource (as recommended in the MultiSource docs).
	// This one is listed last so that if a particular version is available
	// both in one of the above directories _and_ in a remote registry, the
	// local copy will take precedence.
	searchRules = append(searchRules, getproviders.MultiSourceSelector{
		Source: getproviders.NewMemoizeSource(
			getproviders.NewRegistrySource(services, newRegistryHTTPClient(context.TODO())),
		),
		Exclude: directExcluded,
	})

	return getproviders.MultiSource(searchRules)
}

func providerSourceForCLIConfigLocation(loc cliconfig.ProviderInstallationLocation, services *disco.Disco, makeOCICredsPolicy ociCredsPolicyBuilder) (getproviders.Source, tfdiags.Diagnostics) {
	if loc == cliconfig.ProviderInstallationDirect {
		return getproviders.NewMemoizeSource(
			getproviders.NewRegistrySource(services, newRegistryHTTPClient(context.TODO())),
		), nil
	}

	switch loc := loc.(type) {

	case cliconfig.ProviderInstallationFilesystemMirror:
		return getproviders.NewFilesystemMirrorSource(string(loc)), nil

	case cliconfig.ProviderInstallationNetworkMirror:
		url, err := url.Parse(string(loc))
		if err != nil {
			var diags tfdiags.Diagnostics
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid URL for provider installation source",
				fmt.Sprintf("Cannot parse %q as a URL for a network provider mirror: %s.", string(loc), err),
			))
			return nil, diags
		}
		if url.Scheme != "https" || url.Host == "" {
			var diags tfdiags.Diagnostics
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid URL for provider installation source",
				fmt.Sprintf("Cannot use %q as a URL for a network provider mirror: the mirror must be at an https: URL.", string(loc)),
			))
			return nil, diags
		}
		// For historical reasons, we use the registry client timeout for this
		// even though this isn't actually a registry. The other behavior of
		// this client is not suitable for the HTTP mirror source, so we
		// don't use this client directly.
		httpTimeout := newRegistryHTTPClient(context.TODO()).HTTPClient.Timeout
		return getproviders.NewHTTPMirrorSource(url, services.CredentialsSource(), httpTimeout), nil

	case cliconfig.ProviderInstallationOCIMirror:
		mappingFunc := loc.RepositoryMapping
		return getproviders.NewOCIRegistryMirrorSource(
			mappingFunc,
			func(ctx context.Context, registryDomain, repositoryName string) (getproviders.OCIRepositoryStore, error) {
				// We intentionally delay the finalization of the credentials policy until
				// just before we need it because most OpenTofu commands don't install
				// providers at all, and even those that do only need to do this if
				// actually interacting with an OCI mirror, so we can avoid doing
				// this work at all most of the time.
				credsPolicy, err := makeOCICredsPolicy(ctx)
				if err != nil {
					// This deals with only a small number of errors that we can't catch during CLI config validation
					return nil, fmt.Errorf("invalid credentials configuration for OCI registries: %w", err)
				}
				return getOCIRepositoryStore(ctx, registryDomain, repositoryName, credsPolicy)
			},
		), nil

	default:
		// We should not get here because the set of cases above should
		// be comprehensive for all of the
		// cliconfig.ProviderInstallationLocation implementations.
		panic(fmt.Sprintf("unexpected provider source location type %T", loc))
	}
}

func providerDevOverrides(configs []*cliconfig.ProviderInstallation) map[addrs.Provider]getproviders.PackageLocalDir {
	if len(configs) == 0 {
		return nil
	}

	// There should only be zero or one configurations, which is checked by
	// the validation logic in the cliconfig package. Therefore we'll just
	// ignore any additional configurations in here.
	return configs[0].DevOverrides
}
