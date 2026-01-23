// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package providercache

import (
	"context"
	"fmt"
	"log"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/apparentlymart/go-versions/versions"

	"github.com/opentofu/opentofu/internal/addrs"
	copydir "github.com/opentofu/opentofu/internal/copy"
	"github.com/opentofu/opentofu/internal/depsfile"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/tracing"
	"github.com/opentofu/opentofu/internal/tracing/traceattrs"
)

// Installer is the main type in this package, representing a provider installer
// with a particular configuration-specific cache directory and an optional
// global cache directory.
type Installer struct {
	// targetDir is the cache directory we're ultimately aiming to get the
	// requested providers installed into.
	targetDir *Dir

	// source is the provider source that the installer will use to discover
	// what provider versions are available for installation and to
	// find the source locations for any versions that are not already
	// available via one of the cache directories.
	source getproviders.Source

	// globalCacheDir is an optional additional directory that will, if
	// provided, be treated as a read-through cache when retrieving new
	// provider versions. That is, new packages are fetched into this
	// directory first and then linked into targetDir, which allows sharing
	// both the disk space and the download time for a particular provider
	// version between different configurations on the same system.
	globalCacheDir *Dir

	// globalCacheDirMayBreakDependencyLockFile allows a temporary exception to
	// the rule that an entry in globalCacheDir can normally only be used if
	// its validity is already confirmed by an entry in the dependency lock
	// file.
	globalCacheDirMayBreakDependencyLockFile bool

	// builtInProviderTypes is an optional set of types that should be
	// considered valid to appear in the special terraform.io/builtin/...
	// namespace, which we use for providers that are built in to OpenTofu
	// and thus do not need any separate installation step.
	builtInProviderTypes []string

	// unmanagedProviderTypes is a set of provider addresses that should be
	// considered implemented, but that OpenTofu does not manage the
	// lifecycle for, and therefore does not need to worry about the
	// installation of.
	unmanagedProviderTypes map[addrs.Provider]struct{}
}

// NewInstaller constructs and returns a new installer with the given target
// directory and provider source.
//
// A newly-created installer does not have a global cache directory configured,
// but a caller can make a follow-up call to SetGlobalCacheDir to provide
// one prior to taking any installation actions.
//
// The target directory MUST NOT also be an input consulted by the given source,
// or the result is undefined.
func NewInstaller(targetDir *Dir, source getproviders.Source) *Installer {
	return &Installer{
		targetDir: targetDir,
		source:    source,
	}
}

// Clone returns a new Installer which has the a new target directory but
// the same optional global cache directory, the same installation sources,
// and the same built-in/unmanaged providers. The result can be mutated further
// using the various setter methods without affecting the original.
func (i *Installer) Clone(targetDir *Dir) *Installer {
	// For now all of our setter methods just overwrite field values in
	// their entirety, rather than mutating things on the other side of
	// the shared pointers, and so we can safely just shallow-copy the
	// root. We might need to be more careful here if in future we add
	// methods that allow deeper mutations through the stored pointers.
	ret := *i
	ret.targetDir = targetDir
	return &ret
}

// ProviderSource returns the getproviders.Source that the installer would
// use for installing any new providers.
func (i *Installer) ProviderSource() getproviders.Source {
	return i.source
}

// SetGlobalCacheDir activates a second tier of caching for the receiving
// installer, with the given directory used as a read-through cache for
// installation operations that need to retrieve new packages.
//
// The global cache directory for an installer must never be the same as its
// target directory, and must not be used as one of its provider sources.
// If these overlap then undefined behavior will result.
func (i *Installer) SetGlobalCacheDir(cacheDir *Dir) {
	// A little safety check to catch straightforward mistakes where the
	// directories overlap. Better to panic early than to do
	// possibly-destructive actions on the cache directory downstream.
	if same, err := copydir.SameFile(i.targetDir.baseDir, cacheDir.baseDir); err == nil && same {
		panic(fmt.Sprintf("global cache directory %s must not match the installation target directory %s", cacheDir.baseDir, i.targetDir.baseDir))
	}
	i.globalCacheDir = cacheDir
}

// SetGlobalCacheDirMayBreakDependencyLockFile activates or deactivates our
// temporary exception to the rule that the global cache directory can be used
// only when entries are confirmed by existing entries in the dependency lock
// file.
//
// If this is set then if we install a provider for the first time from the
// cache then the dependency lock file will include only the checksum from
// the package in the global cache, which means the lock file won't be portable
// to OpenTofu running on another operating system or CPU architecture.
func (i *Installer) SetGlobalCacheDirMayBreakDependencyLockFile(mayBreak bool) {
	i.globalCacheDirMayBreakDependencyLockFile = mayBreak
}

// HasGlobalCacheDir returns true if someone has previously called
// SetGlobalCacheDir to configure a global cache directory for this installer.
func (i *Installer) HasGlobalCacheDir() bool {
	return i.globalCacheDir != nil
}

// SetBuiltInProviderTypes tells the receiver to consider the type names in the
// given slice to be valid as providers in the special special
// terraform.io/builtin/... namespace that we use for providers that are
// built in to OpenTofu and thus do not need a separate installation step.
//
// If a caller requests installation of a provider in that namespace, the
// installer will treat it as a no-op if its name exists in this list, but
// will produce an error if it does not.
//
// The default, if this method isn't called, is for there to be no valid
// builtin providers.
//
// Do not modify the buffer under the given slice after passing it to this
// method.
func (i *Installer) SetBuiltInProviderTypes(types []string) {
	i.builtInProviderTypes = types
}

// SetUnmanagedProviderTypes tells the receiver to consider the providers
// indicated by the passed addrs.Providers as unmanaged. OpenTofu does not
// need to control the lifecycle of these providers, and they are assumed to be
// running already when OpenTofu is started. Because these are essentially
// processes, not binaries, OpenTofu will not do any work to ensure presence
// or versioning of these binaries.
func (i *Installer) SetUnmanagedProviderTypes(types map[addrs.Provider]struct{}) {
	i.unmanagedProviderTypes = types
}

// EnsureProviderVersions compares the given provider requirements with what
// is already available in the installer's target directory and then takes
// appropriate installation actions to ensure that suitable packages
// are available in the target cache directory.
//
// The given mode modifies how the operation will treat providers that already
// have acceptable versions available in the target cache directory. See the
// documentation for InstallMode and the InstallMode values for more
// information.
//
// The given context can be used to cancel the overall installation operation
// (causing any operations in progress to fail with an error), and can also
// include an InstallerEvents value for optional intermediate progress
// notifications.
//
// If a given InstallerEvents subscribes to notifications about installation
// failures then those notifications will be redundant with the ones included
// in the final returned error value so callers should show either one or the
// other, and not both.
func (i *Installer) EnsureProviderVersions(ctx context.Context, locks *depsfile.Locks, reqs getproviders.Requirements, mode InstallMode) (*depsfile.Locks, error) {
	ctx, span := tracing.Tracer().Start(ctx, "Install Providers") // TODO: Discuss span name
	defer span.End()

	evts := installerEventsForContext(ctx)

	// Whenever possible we prefer to collect separate errors for each
	// problematic provider and then report them all together at the end,
	// because that can allow an operator to notice a systematic problem
	// across multiple providers, such as a particular registry failing
	// in the same way regardless of which provider is requested.
	//
	// The other functions we call below will gradually add errors here
	// as appropriate. Those functions only return an err directly
	// themselves in situations that are not related to any particular
	// provider and so prevent us from continuing further at all.
	errs := map[addrs.Provider]error{}

	// We'll work with a copy of the given locks, so we can modify it and
	// return the updated locks without affecting the caller's object.
	// We'll add, replace, or remove locks in here during our work so that the
	// final locks file reflects what the installer has selected.
	locks = locks.DeepCopy()

	if cb := evts.PendingProviders; cb != nil {
		cb(reqs)
	}

	// Step 1: Which providers might we need to fetch a new version of?
	// This produces the subset of requirements we need to ask the provider
	// source about. If we're in the normal (non-upgrade) mode then we'll
	// just ask the source to confirm the continued existence of what
	// was locked, or otherwise we'll find the newest version matching the
	// configured version constraint.
	mightNeed, locked := i.ensureProviderVersionsMightNeed(ctx, locks, reqs, mode, errs)

	// Step 2: Query the provider source for each of the providers we selected
	// in the first step and select the latest available version that is
	// in the set of acceptable versions.
	//
	// This produces a set of packages to install to our cache in the next step.
	need, err := i.ensureProviderVersionsNeed(ctx, locks, reqs, mightNeed, locked, errs)
	if err != nil {
		return nil, err
	}

	// Step 3: For each provider version we've decided we need to install,
	// install its package into our target cache (possibly via the global cache).
	targetPlatform := i.targetDir.targetPlatform // we inherit this to behave correctly in unit tests
	span.SetAttributes(traceattrs.OpenTofuTargetPlatform(targetPlatform.String()))
	span.SetName("Install Providers - " + targetPlatform.String())
	authResults, err := i.ensureProviderVersionsInstall(ctx, locks, reqs, mode, need, targetPlatform, errs)
	if err != nil {
		return nil, err
	}

	// Emit final event for fetching if any were successfully fetched
	if cb := evts.ProvidersAuthenticated; cb != nil && len(authResults) > 0 {
		cb(authResults)
	}

	// Finally, if the lock structure contains locks for any providers that
	// are no longer needed by this configuration, we'll remove them. This
	// is important because we will not have installed those providers
	// above and so a lock file still containing them would make the working
	// directory invalid: not every provider in the lock file is available
	// for use.
	for providerAddr := range locks.AllProviders() {
		if _, ok := reqs[providerAddr]; !ok {
			locks.RemoveProvider(providerAddr)
		}
	}

	if len(errs) > 0 {
		return locks, InstallerError{
			ProviderErrors: errs,
		}
	}
	return locks, nil
}

func (i *Installer) ensureProviderVersionsMightNeed(
	ctx context.Context,
	locks *depsfile.Locks,
	reqs getproviders.Requirements,
	mode InstallMode,
	errs map[addrs.Provider]error,
) (
	map[addrs.Provider]getproviders.VersionSet,
	map[addrs.Provider]bool,
) {
	evts := installerEventsForContext(ctx)
	mightNeed := map[addrs.Provider]getproviders.VersionSet{}
	locked := map[addrs.Provider]bool{}

	for provider, versionConstraints := range reqs {
		if provider.IsBuiltIn() {
			// Built in providers do not require installation but we'll still
			// verify that the requested provider name is valid.
			valid := false
			for _, name := range i.builtInProviderTypes {
				if name == provider.Type {
					valid = true
					break
				}
			}
			var err error
			if valid {
				if len(versionConstraints) == 0 {
					// Other than reporting an event for the outcome of this
					// provider, we'll do nothing else with it: it's just
					// automatically available for use.
					if cb := evts.BuiltInProviderAvailable; cb != nil {
						cb(provider)
					}
				} else {
					// A built-in provider is not permitted to have an explicit
					// version constraint, because we can only use the version
					// that is built in to the current OpenTofu release.
					err = fmt.Errorf("built-in providers do not support explicit version constraints")
				}
			} else {
				err = fmt.Errorf("this OpenTofu release has no built-in provider named %q", provider.Type)
			}
			if err != nil {
				errs[provider] = err
				if cb := evts.BuiltInProviderFailure; cb != nil {
					cb(provider, err)
				}
			}
			continue
		}
		if _, ok := i.unmanagedProviderTypes[provider]; ok {
			// unmanaged providers do not require installation
			continue
		}
		acceptableVersions := versions.MeetingConstraints(versionConstraints)
		if !mode.forceQueryAllProviders() {
			// If we're not forcing potential changes of version then an
			// existing selection from the lock file takes priority over
			// the currently-configured version constraints.
			if lock := locks.Provider(provider); lock != nil {
				if !acceptableVersions.Has(lock.Version()) {
					err := fmt.Errorf(
						"locked provider %s %s does not match configured version constraint %s; must use tofu init -upgrade to allow selection of new versions",
						provider, lock.Version(), getproviders.VersionConstraintsString(versionConstraints),
					)
					errs[provider] = err
					// This is a funny case where we're returning an error
					// before we do any querying at all. To keep the event
					// stream consistent without introducing an extra event
					// type, we'll emit an artificial QueryPackagesBegin for
					// this provider before we indicate that it failed using
					// QueryPackagesFailure.
					if cb := evts.QueryPackagesBegin; cb != nil {
						cb(provider, versionConstraints, true)
					}
					if cb := evts.QueryPackagesFailure; cb != nil {
						cb(provider, err)
					}
					continue
				}
				acceptableVersions = versions.Only(lock.Version())
				locked[provider] = true
			}
		}
		mightNeed[provider] = acceptableVersions
	}

	return mightNeed, locked
}

func (i *Installer) ensureProviderVersionsNeed(
	ctx context.Context,
	locks *depsfile.Locks,
	reqs getproviders.Requirements,
	mightNeed map[addrs.Provider]getproviders.VersionSet,
	locked map[addrs.Provider]bool,
	errs map[addrs.Provider]error,
) (map[addrs.Provider]getproviders.Version, error) {
	evts := installerEventsForContext(ctx)

	if err := ctx.Err(); err != nil {
		// If our context has been cancelled or reached a timeout then
		// we'll abort early, because subsequent operations against
		// that context will fail immediately anyway.
		return nil, err
	}

	computeNeeds := func(provider addrs.Provider, acceptableVersions getproviders.VersionSet) (getproviders.Version, error) {
		if cb := evts.QueryPackagesBegin; cb != nil {
			cb(provider, reqs[provider], locked[provider])
		}
		// Version 0.0.0 not supported
		if err := checkUnspecifiedVersion(acceptableVersions); err != nil {
			if cb := evts.QueryPackagesFailure; cb != nil {
				cb(provider, err)
			}
			return getproviders.Version{}, err
		}

		available, warnings, err := i.source.AvailableVersions(ctx, provider)
		if err != nil {
			if cb := evts.QueryPackagesFailure; cb != nil {
				cb(provider, err)
			}
			// We will take no further actions for this provider.
			return getproviders.Version{}, err
		}
		if len(warnings) > 0 {
			if cb := evts.QueryPackagesWarning; cb != nil {
				cb(provider, warnings)
			}
		}
		available.Sort()                           // put the versions in increasing order of precedence
		for i := len(available) - 1; i >= 0; i-- { // walk backwards to consider newer versions first
			if acceptableVersions.Has(available[i]) {
				if cb := evts.QueryPackagesSuccess; cb != nil {
					cb(provider, available[i])
				}
				return available[i], nil
			}
		}
		// If we get here then the source has no packages that meet the given
		// version constraint, which we model as a query error.
		if locked[provider] {
			// This situation should be a rare one: it suggests that a
			// version was previously available but was yanked for some
			// reason.
			lock := locks.Provider(provider)
			err = fmt.Errorf("the previously-selected version %s is no longer available", lock.Version())
		} else {
			err = fmt.Errorf("no available releases match the given constraints %s", getproviders.VersionConstraintsString(reqs[provider]))
			log.Printf("[DEBUG] %s", err.Error())
			log.Printf("[DEBUG] Available releases: %s", available)
		}
		if cb := evts.QueryPackagesFailure; cb != nil {
			cb(provider, err)
		}
		return getproviders.Version{}, err
	}

	need := map[addrs.Provider]getproviders.Version{}
	var updateLock sync.Mutex
	var wg sync.WaitGroup

	for provider, acceptableVersions := range mightNeed {
		wg.Go(func() {
			// Heavy lifting
			version, err := computeNeeds(provider, acceptableVersions)

			// Update results
			updateLock.Lock()
			defer updateLock.Unlock()

			if err != nil {
				errs[provider] = err
			} else {
				need[provider] = version
			}
		})
	}
	wg.Wait()

	return need, nil
}

func (i *Installer) ensureProviderVersionsInstall(
	ctx context.Context,
	locks *depsfile.Locks,
	reqs getproviders.Requirements,
	mode InstallMode,
	need map[addrs.Provider]getproviders.Version,
	targetPlatform getproviders.Platform,
	errs map[addrs.Provider]error,
) (map[addrs.Provider]*getproviders.PackageAuthenticationResult, error) {
	if err := ctx.Err(); err != nil {
		// If our context has been cancelled or reached a timeout then
		// we'll abort early, because subsequent operations against
		// that context will fail immediately anyway.
		return nil, err
	}

	authResults := map[addrs.Provider]*getproviders.PackageAuthenticationResult{} // record auth results for all successfully fetched providers
	var updateLock sync.Mutex
	var wg sync.WaitGroup

	providerExistingLock := func(provider addrs.Provider) *depsfile.ProviderLock {
		updateLock.Lock()
		defer updateLock.Unlock()
		return locks.Provider(provider)
	}
	for provider, version := range need {
		wg.Go(func() {
			traceCtx, span := tracing.Tracer().Start(ctx,
				fmt.Sprintf("Install Provider %q", provider.String()),
				tracing.SpanAttributes(
					traceattrs.OpenTofuProviderAddress(provider.String()),
					traceattrs.OpenTofuProviderVersion(version.String()),
					traceattrs.OpenTofuTargetPlatform(targetPlatform.String()),
				),
			)
			defer span.End()

			// Heavy lifting
			authResult, newHashes, err := i.ensureProviderVersionInstalled(traceCtx, providerExistingLock(provider), mode, provider, version, targetPlatform)

			// Update results
			updateLock.Lock()
			defer updateLock.Unlock()

			if err != nil {
				tracing.SetSpanError(span, err)
				errs[provider] = err
			}

			if authResult != nil {
				authResults[provider] = authResult
			}
			if len(newHashes) > 0 {
				locks.SetProvider(provider, version, reqs[provider], newHashes)
			}
		})
	}

	wg.Wait()

	return authResults, nil
}

func (i *Installer) ensureProviderVersionInstalled(
	ctx context.Context,
	lock *depsfile.ProviderLock,
	mode InstallMode,
	provider addrs.Provider,
	version getproviders.Version,
	targetPlatform getproviders.Platform,
) (*getproviders.PackageAuthenticationResult, []getproviders.Hash, error) {
	evts := installerEventsForContext(ctx)

	var preferredHashes []getproviders.Hash
	if lock != nil && lock.Version() == version { // hash changes are expected if the version is also changing
		preferredHashes = lock.PreferredHashes()
	}

	// If our target directory already has the provider version that fulfills the lock file, carry on
	if installed := i.targetDir.ProviderVersion(provider, version); installed != nil {
		if len(preferredHashes) > 0 {
			if matches, _ := installed.MatchesAnyHash(preferredHashes); matches {
				if cb := evts.ProviderAlreadyInstalled; cb != nil {
					cb(provider, version, false)
				}
				// Even though the package is installed, the requirements in the lockfile may still need to be updated
				return nil, lock.AllHashes(), nil
			}
		}
	}

	var installTo, linkTo *Dir
	if i.globalCacheDir != nil {
		installTo = i.globalCacheDir
		linkTo = i.targetDir
	} else {
		installTo = i.targetDir
		linkTo = nil // no linking needed
	}

	result, newHashes, err := i.ensureProviderVersionInDirectory(ctx, lock, mode, provider, version, targetPlatform, installTo)

	if err != nil {
		return result, newHashes, err
	}

	if linkTo != nil {
		if cb := evts.LinkFromCacheBegin; cb != nil {
			cb(provider, version, installTo.BasePath())
		}

		// We don't do a hash check here because we already did that
		// as part of the ensureProviderVersionInDirectory call above.
		new := installTo.ProviderVersion(provider, version)
		err := linkTo.LinkFromOtherCache(ctx, new, nil)
		if err != nil {
			if cb := evts.LinkFromCacheFailure; cb != nil {
				cb(provider, version, err)
			}
			return nil, nil, err
		}

		// We should now also find the package in the linkTo dir, which
		// gives us the final value of "new" where the path points in to
		// the true target directory, rather than possibly the global
		// cache directory.
		new = linkTo.ProviderVersion(provider, version)
		if new == nil {
			err := fmt.Errorf("after installing %s it is still not detected in %s; this is a bug in OpenTofu", provider, linkTo.BasePath())
			if cb := evts.LinkFromCacheFailure; cb != nil {
				cb(provider, version, err)
			}
			return nil, nil, err
		}
		if _, err := new.ExecutableFile(); err != nil {
			err := fmt.Errorf("provider binary not found: %w", err)
			if cb := evts.LinkFromCacheFailure; cb != nil {
				cb(provider, version, err)
			}
			return nil, nil, err
		}

		if cb := evts.LinkFromCacheSuccess; cb != nil {
			cb(provider, version, new.PackageDir)
		}
	}

	return result, newHashes, err
}

func (i *Installer) ensureProviderVersionInDirectory(
	ctx context.Context,
	lock *depsfile.ProviderLock,
	mode InstallMode,
	provider addrs.Provider,
	version getproviders.Version,
	targetPlatform getproviders.Platform,
	installTo *Dir,
) (*getproviders.PackageAuthenticationResult, []getproviders.Hash, error) {
	evts := installerEventsForContext(ctx)

	var preferredHashes []getproviders.Hash
	if lock != nil && lock.Version() == version { // hash changes are expected if the version is also changing
		preferredHashes = lock.PreferredHashes()
	}

	isGlobalCache := installTo == i.globalCacheDir

	// If our target directory already has the provider version that fulfills the lock file, carry on
	if installed := installTo.ProviderVersion(provider, version); installed != nil {
		if len(preferredHashes) > 0 {
			if matches, _ := installed.MatchesAnyHash(preferredHashes); matches {
				if cb := evts.ProviderAlreadyInstalled; cb != nil {
					cb(provider, version, isGlobalCache)
				}

				// Even though the package is installed, the requirements in the lockfile may still need to be updated
				return nil, lock.AllHashes(), nil
			}
		}
	}

	// Step 3b: Get the package metadata for the selected version from our
	// provider source.
	//
	// This is the step where we might detect and report that the provider
	// isn't available for the current platform.
	if cb := evts.FetchPackageMeta; cb != nil {
		cb(provider, version)
	}
	meta, err := i.source.PackageMeta(ctx, provider, version, targetPlatform)
	if err != nil {
		if cb := evts.FetchPackageFailure; cb != nil {
			cb(provider, version, err)
		}
		return nil, nil, err
	}

	// Step 3c: Retrieve the package indicated by the metadata we received,
	// either directly into our target directory or via the global cache
	// directory.
	if cb := evts.FetchPackageBegin; cb != nil {
		cb(provider, version, meta.Location, isGlobalCache)
	}

	allowedHashes := preferredHashes
	if mode.forceInstallChecksums() {
		allowedHashes = []getproviders.Hash{}
	}

	allowSkippingInstallWithoutHashes := i.globalCacheDirMayBreakDependencyLockFile && isGlobalCache
	authResult, err := installTo.InstallPackage(ctx, meta, allowedHashes, allowSkippingInstallWithoutHashes)
	if err != nil {
		// TODO: Consider retrying for certain kinds of error that seem
		// likely to be transient. For now, we just treat all errors equally.
		if cb := evts.FetchPackageFailure; cb != nil {
			cb(provider, version, err)
		}
		return nil, nil, err
	}

	new := installTo.ProviderVersion(provider, version)
	if new == nil {
		err := fmt.Errorf("after installing %s it is still not detected in %s; this is a bug in OpenTofu", provider, installTo.BasePath())
		if cb := evts.FetchPackageFailure; cb != nil {
			cb(provider, version, err)
		}
		return nil, nil, err
	}
	if _, err := new.ExecutableFile(); err != nil {
		err := fmt.Errorf("provider binary not found: %w", err)
		if cb := evts.FetchPackageFailure; cb != nil {
			cb(provider, version, err)
		}
		return nil, nil, err
	}

	// The InstallPackage call above should've verified that
	// the package matches one of the hashes previously recorded,
	// if any. We'll now augment those hashes with a new set populated
	// with the hashes returned by the upstream source and from the
	// package we've just installed, which allows the lock file to
	// gradually transition to newer hash schemes when they become
	// available.
	//
	// This is assuming that if a package matches both a hash we saw before
	// _and_ a new hash then the new hash is a valid substitute for
	// the previous hash.
	//
	// The hashes slice gets deduplicated in the lock file
	// implementation, so we don't worry about potentially
	// creating duplicates here.
	var priorHashes []getproviders.Hash
	if lock != nil && lock.Version() == version {
		// If the version we're installing is identical to the
		// one we previously locked then we'll keep all of the
		// hashes we saved previously and add to it. Otherwise
		// we'll be starting fresh, because each version has its
		// own set of packages and thus its own hashes.
		priorHashes = append(priorHashes, preferredHashes...)
	}
	newHash, err := new.Hash()
	if err != nil {
		err := fmt.Errorf("after installing %s, failed to compute a checksum for it: %w", provider, err)
		if cb := evts.FetchPackageFailure; cb != nil {
			cb(provider, version, err)
		}
		return authResult, nil, err
	}

	// localHashes is the set of hashes that we were able to verify locally
	// based on the data we downloaded.
	localHashes := slices.Collect(authResult.HashesWithDisposition(func(hd *getproviders.HashDisposition) bool {
		return hd.VerifiedLocally
	}))
	localHashes = append(localHashes, newHash) // the hash we calculated above was _also_ verified locally

	// We have different rules for what subset of hashes we track in
	// the dependency lock file depending on the provider. Refer to
	// the documentation of the following function for more information.
	signingRequired := getproviders.ShouldEnforceGPGValidationForProvider(provider)
	signedHashes := slices.Collect(authResult.HashesWithDisposition(func(hd *getproviders.HashDisposition) bool {
		if !signingRequired {
			// When signing isn't required, we pretend that anything
			// that was reported by the origin registry was "signed",
			// just for the purposes of updating the lock file and
			// reporting that lock file update to the UI layer through
			// the evts object.
			// Note that the "tofu init" UI relies on us pretending
			// that these are "signed" to avoid generating its warning
			// that the dependency lock file might be incomplete.
			return hd.ReportedByRegistry
		}
		return hd.SignedByAnyGPGKeys()
	}))

	var newHashes []getproviders.Hash
	newHashes = append(newHashes, newHash)
	newHashes = append(newHashes, priorHashes...)
	newHashes = append(newHashes, localHashes...)
	newHashes = append(newHashes, signedHashes...)

	if cb := evts.ProvidersLockUpdated; cb != nil {
		// priorHashes is already sorted, but we do need to sort
		// the newly-generated localHashes and signedHashes.
		sort.Slice(localHashes, func(i, j int) bool {
			return localHashes[i].String() < localHashes[j].String()
		})
		sort.Slice(signedHashes, func(i, j int) bool {
			return signedHashes[i].String() < signedHashes[j].String()
		})
		// these slices might also contain duplicates if the
		// same hash was found in two different ways, so we'll
		// adjust for that. This relies on the sorting above
		// and modifies the underlying arrays in-place.
		localHashes = slices.Compact(localHashes)
		signedHashes = slices.Compact(signedHashes)
		cb(provider, version, localHashes, signedHashes, priorHashes)
	}

	if cb := evts.FetchPackageSuccess; cb != nil {
		cb(provider, version, new.PackageDir, authResult)
	}

	return authResult, newHashes, nil
}

// checkUnspecifiedVersion Check the presence of version 0.0.0 and return an error with a tip
func checkUnspecifiedVersion(acceptableVersions versions.Set) error {
	if !acceptableVersions.Exactly(versions.Unspecified) {
		return nil
	}
	tip := "If the version 0.0.0 is intended to represent a non-published provider, consider using dev_overrides - https://opentofu.org/docs/cli/config/config-file/#development-overrides-for-provider-developers"
	return fmt.Errorf("0.0.0 is not a valid provider version. \n%s", tip)
}

// InstallMode customizes the details of how an install operation treats
// providers that have versions already cached in the target directory.
type InstallMode rune

const (
	// InstallNewProvidersOnly is an InstallMode that causes the installer
	// to accept any existing version of a requested provider that is already
	// cached as long as it's in the given version sets, without checking
	// whether new versions are available that are also in the given version
	// sets.
	InstallNewProvidersOnly InstallMode = 'N'

	// InstallNewProvidersForce is an InstallMode that follows the same
	// logic as InstallNewProvidersOnly except it does not verify existing
	// checksums but force installs new checksums for all given providers.
	InstallNewProvidersForce InstallMode = 'F'

	// InstallUpgrades is an InstallMode that causes the installer to check
	// all requested providers to see if new versions are available that
	// are also in the given version sets, even if a suitable version of
	// a given provider is already available.
	InstallUpgrades InstallMode = 'U'
)

func (m InstallMode) forceQueryAllProviders() bool {
	return m == InstallUpgrades
}

func (m InstallMode) forceInstallChecksums() bool {
	return m == InstallNewProvidersForce
}

// InstallerError is an error type that may be returned (but is not guaranteed)
// from Installer.EnsureProviderVersions to indicate potentially several
// separate failed installation outcomes for different providers included in
// the overall request.
type InstallerError struct {
	ProviderErrors map[addrs.Provider]error
}

func (err InstallerError) Error() string {
	addrs := make([]addrs.Provider, 0, len(err.ProviderErrors))
	for addr := range err.ProviderErrors {
		addrs = append(addrs, addr)
	}
	sort.Slice(addrs, func(i, j int) bool {
		return addrs[i].LessThan(addrs[j])
	})
	var b strings.Builder
	b.WriteString("some providers could not be installed:\n")
	for _, addr := range addrs {
		providerErr := err.ProviderErrors[addr]
		fmt.Fprintf(&b, "- %s: %s\n", addr, providerErr)
	}
	return strings.TrimSpace(b.String())
}
