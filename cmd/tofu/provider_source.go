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
	"github.com/hashicorp/terraform-svchost/disco"
	orasRemote "oras.land/oras-go/v2/registry/remote"
	orasAuth "oras.land/oras-go/v2/registry/remote/auth"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/cliconfig"
	"github.com/opentofu/opentofu/internal/command/cliconfig/ociauthconfig"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ociCredsPolicyBuilder is the type of a callback function that the [providerSource]
// functions will use if any of the configured provider installation methods
// need to interact with OCI Distribution registries.
//
// We represent this indirectly as a callback function so that we can skip doing
// this work in the common case where we won't need to interact with OCI registries
// at all.
type ociCredsPolicyBuilder func(context.Context) (ociauthconfig.CredentialsConfigs, error)

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
			getproviders.NewRegistrySource(services),
		),
		Exclude: directExcluded,
	})

	return getproviders.MultiSource(searchRules)
}

func providerSourceForCLIConfigLocation(loc cliconfig.ProviderInstallationLocation, services *disco.Disco, makeOCICredsPolicy ociCredsPolicyBuilder) (getproviders.Source, tfdiags.Diagnostics) {
	if loc == cliconfig.ProviderInstallationDirect {
		return getproviders.NewMemoizeSource(
			getproviders.NewRegistrySource(services),
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
		return getproviders.NewHTTPMirrorSource(url, services.CredentialsSource()), nil

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

// getOCIRepositoryStore instantiates a [getproviders.OCIRepositoryStore] implementation to use
// when accessing the given repository on the given registry, using the given OCI credentials
// policy to decide which credentials to use.
func getOCIRepositoryStore(ctx context.Context, registryDomain, repositoryName string, credsPolicy ociauthconfig.CredentialsConfigs) (getproviders.OCIRepositoryStore, error) {
	// We currently use the ORAS-Go library to satisfy the [getproviders.OCIRepositoryStore]
	// interface, which is easy because that interface was designed to match a subset of
	// the ORAS-Go API since we had no particular need to diverge from it. However, we consider
	// ORAS-Go to be an implementation detail here and so we should avoid any ORAS-Go
	// types becoming part of the direct public API between packages.

	// ORAS-Go has a bit of an impedence mismatch with us in that it thinks of credentials
	// as being a per-registry thing rather than a per-repository thing, so we deal with
	// the credSource resolution ourselves here and then just return whatever we found to
	// ORAS when it asks through its callback. In practice we only interact with one
	// repository per client so this is just a little inconvenient and not a practical problem.
	credSource, err := credsPolicy.CredentialsSourceForRepository(ctx, registryDomain, repositoryName)
	if ociauthconfig.IsCredentialsNotFoundError(err) {
		credSource = nil // we'll just try without any credentials, then
	} else if err != nil {
		return nil, fmt.Errorf("finding credentials for %q: %w", registryDomain, err)
	}
	client := &orasAuth.Client{
		Client: httpclient.New(), // the underlying HTTP client to use, preconfigured with OpenTofu's User-Agent string
		Credential: func(ctx context.Context, hostport string) (orasAuth.Credential, error) {
			if hostport != registryDomain {
				// We should not send the credentials we selected to any registry
				// other than the one they were configured for.
				return orasAuth.EmptyCredential, nil
			}
			if credSource == nil {
				return orasAuth.EmptyCredential, nil
			}
			creds, err := credSource.Credentials(ctx, ociCredentialsLookupEnv{})
			if ociauthconfig.IsCredentialsNotFoundError(err) {
				return orasAuth.EmptyCredential, nil
			}
			if err != nil {
				return orasAuth.Credential{}, err
			}
			return creds.ToORASCredential(), nil
		},
		Cache: orasAuth.NewCache(),
	}
	reg, err := orasRemote.NewRegistry(registryDomain)
	if err != nil {
		return nil, err // This is only for registryDomain validation errors, and we should've caught those much earlier than here
	}
	reg.Client = client
	err = reg.Ping(ctx) // tests whether the given domain refers to a valid OCI repository and will accept the credentials
	if err != nil {
		return nil, fmt.Errorf("failed to contact OCI registry at %q: %w", registryDomain, err)
	}
	repo, err := reg.Repository(ctx, repositoryName)
	if err != nil {
		return nil, err // This is only for repositoryName validation errors, and we should've caught those much earlier than here
	}
	// NOTE: At this point we don't yet know if the named repository actually exists
	// in the registry. The caller will find that out when they try to interact
	// with the methods of [getproviders.OCIRepositoryStore].
	return repo, nil
}

// ociCredentialsLookupEnv is our implementation of ociauthconfig.CredentialsLookupEnvironment
// used when resolving the selected credentials for a particular OCI repository.
type ociCredentialsLookupEnv struct{}

var _ ociauthconfig.CredentialsLookupEnvironment = ociCredentialsLookupEnv{}

// QueryDockerCredentialHelper implements ociauthconfig.CredentialsLookupEnvironment.
func (o ociCredentialsLookupEnv) QueryDockerCredentialHelper(ctx context.Context, helperName string, serverURL string) (ociauthconfig.DockerCredentialHelperGetResult, error) {
	// TODO: Implement this
	return ociauthconfig.DockerCredentialHelperGetResult{}, fmt.Errorf("support for Docker-style credential helpers is not yet available")
}
