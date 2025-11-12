// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package providercache

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/flock"
	"github.com/opentofu/opentofu/internal/getproviders"
)

// InstallPackage takes a metadata object describing a package available for
// installation, retrieves that package, and installs it into the receiving
// cache directory.
//
// If the allowedHashes set has non-zero length then at least one of the hashes
// in the set must match the package that "entry" refers to. If none of the
// hashes match then the returned error message assumes that the hashes came
// from a lock file.
func (d *Dir) InstallPackage(ctx context.Context, meta getproviders.PackageMeta, allowedHashes []getproviders.Hash, allowSkippingInstallWithoutHashes bool) (*getproviders.PackageAuthenticationResult, error) {
	unlock, err := d.lock(ctx, meta.Provider, meta.Version)
	if err != nil {
		return nil, err
	}

	install, installErr := d.installPackageWithLock(ctx, meta, allowedHashes, allowSkippingInstallWithoutHashes)

	return install, errors.Join(installErr, unlock())
}
func (d *Dir) installPackageWithLock(ctx context.Context, meta getproviders.PackageMeta, allowedHashes []getproviders.Hash, allowSkippingInstallWithoutHashes bool) (*getproviders.PackageAuthenticationResult, error) {
	if meta.TargetPlatform != d.targetPlatform {
		return nil, fmt.Errorf("can't install %s package into cache directory expecting %s", meta.TargetPlatform, d.targetPlatform)
	}
	newPath := getproviders.UnpackedDirectoryPathForPackage(
		d.baseDir, meta.Provider, meta.Version, d.targetPlatform,
	)

	log.Printf("[TRACE] providercache.Dir.InstallPackage: installing %s v%s from %s", meta.Provider, meta.Version, meta.Location)

	// Check to see if it is already installed
	if entry := d.ProviderVersion(meta.Provider, meta.Version); entry != nil {
		if allowSkippingInstallWithoutHashes && len(allowedHashes) == 0 {
			// Ensure that the provider exists
			if _, err := entry.ExecutableFile(); err == nil {
				return nil, nil
			}
		}

		if matches, err := entry.MatchesAnyHash(allowedHashes); err == nil && matches {
			// No auth result needed, package is valid
			return getproviders.NewPackageHashAuthentication(meta.TargetPlatform, allowedHashes).AuthenticatePackage(entry.PackageLocation())
		}
	}

	return meta.Location.InstallProviderPackage(ctx, meta, newPath, allowedHashes)
}

// LinkFromOtherCache takes a CachedProvider value produced from another Dir
// and links it into the cache represented by the receiver Dir.
//
// This is used to implement tiered caching, where new providers are first
// populated into a system-wide shared cache and then linked from there into
// a configuration-specific local cache.
//
// It's invalid to link a CachedProvider from a particular Dir into that same
// Dir, because that would otherwise potentially replace a real package
// directory with a circular link back to itself.
//
// If the allowedHashes set has non-zero length then at least one of the hashes
// in the set must match the package that "entry" refers to. If none of the
// hashes match then the returned error message assumes that the hashes came
// from a lock file.
func (d *Dir) LinkFromOtherCache(ctx context.Context, entry *CachedProvider, allowedHashes []getproviders.Hash) error {
	if len(allowedHashes) > 0 {
		if matches, err := entry.MatchesAnyHash(allowedHashes); err != nil {
			return fmt.Errorf(
				"failed to calculate checksum for cached copy of %s %s in %s: %w",
				entry.Provider, entry.Version, d.baseDir, err,
			)
		} else if !matches {
			return fmt.Errorf(
				"the provider cache at %s has a copy of %s %s that doesn't match any of the checksums recorded in the dependency lock file",
				d.baseDir, entry.Provider, entry.Version,
			)
		}
	}

	newPath := getproviders.UnpackedDirectoryPathForPackage(
		d.baseDir, entry.Provider, entry.Version, d.targetPlatform,
	)
	currentPath := entry.PackageDir
	log.Printf("[TRACE] providercache.Dir.LinkFromOtherCache: linking %s v%s from existing cache %s to %s", entry.Provider, entry.Version, currentPath, newPath)

	// We re-use the process of installing from a local directory here, because
	// the two operations are fundamentally the same: symlink if possible,
	// deep-copy otherwise.
	meta := getproviders.PackageMeta{
		Provider: entry.Provider,
		Version:  entry.Version,

		// FIXME: How do we populate this?
		ProtocolVersions: nil,
		TargetPlatform:   d.targetPlatform,

		// Because this is already unpacked, the filename is synthetic
		// based on the standard naming scheme.
		Filename: fmt.Sprintf("terraform-provider-%s_%s_%s.zip",
			entry.Provider.Type, entry.Version, d.targetPlatform),
		Location: getproviders.PackageLocalDir(currentPath),
	}
	// No further hash check here because we already checked the hash
	// of the source directory above.
	_, err := meta.Location.InstallProviderPackage(ctx, meta, newPath, nil)
	return err
}

func (d *Dir) lock(ctx context.Context, provider addrs.Provider, version getproviders.Version) (func() error, error) {
	providerPath := getproviders.UnpackedDirectoryPathForPackage(d.baseDir, provider, version, d.targetPlatform)

	// If the lockfile is put within the target directory, it can mess with hashing
	// Instead we add a suffix to the last part of the path (targetplatform) and lock that file instead.
	dirPath := filepath.Dir(providerPath)
	lockFileName := filepath.Base(providerPath) + ".lock"
	lockFile := filepath.Join(dirPath, lockFileName)

	log.Printf("[TRACE] Attempting to acquire global provider lock %s", lockFile)

	// Ensure the provider directory exists
	//nolint: mnd // directory permissions
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return nil, err
	}

	// Try to get a handle to the file (or create if it does not exist)
	// They will all end up with the same file handle on any correctly implemented filesystem.
	// This is one of the many reasons we recommend users look at the flock support of their
	// networked filesystems when using the global provider cache.
	// Windows: even though out flock creates an exclusive lock, we are still able to open a handle to this file and wait below for the actual lock to be provided.
	f, err := os.OpenFile(lockFile, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	// If the callers InstallerEvents has a hook function for
	// CacheDirLockContended then we'll notify it if we take more than five
	// seconds to acquire the lock, to give some feedback about what's causing
	// delay here. 5 seconds is an arbitrary amount that's short enough to
	// give relatively prompt feedback but long enough to be reasonably
	// confident that a delay here is caused by lock contention.
	evts := installerEventsForContext(ctx)
	if evts.CacheDirLockContended != nil {
		cancelWhenSlow := whenSlow(5*time.Second, func() {
			evts.CacheDirLockContended(d.BasePath())
		})
		defer cancelWhenSlow()
	}

	err = flock.LockBlocking(ctx, f)
	if err != nil {
		// Ensure that we are not in a partially failed state
		return nil, fmt.Errorf("unable to acquire file lock on %q: %w", lockFile, err)
	}

	return func() error {
		log.Printf("[TRACE] Releasing global provider lock %s", lockFile)

		unlockErr := flock.Unlock(f)

		// Prefer close error over unlock error
		err := f.Close()
		if err != nil {
			return err
		}
		return unlockErr
	}, nil
}

func whenSlow(dur time.Duration, f func()) (cancel func()) {
	cancelCh := make(chan struct{})
	go func() {
		select {
		case <-cancelCh:
		case <-time.After(dur):
			f()
		}
	}()
	return func() {
		close(cancelCh)
	}
}
