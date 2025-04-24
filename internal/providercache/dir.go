// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package providercache

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/flock"
	"github.com/opentofu/opentofu/internal/getproviders"
)

// Dir represents a single local filesystem directory containing cached
// provider plugin packages that can be both read from (to find providers to
// use for operations) and written to (during provider installation).
//
// The contents of a cache directory follow the same naming conventions as a
// getproviders.FilesystemMirrorSource, except that the packages are always
// kept in the "unpacked" form (a directory containing the contents of the
// original distribution archive) so that they are ready for direct execution.
//
// A Dir also pays attention only to packages for the current host platform,
// silently ignoring any cached packages for other platforms.
//
// Various Dir methods return values that are technically mutable due to the
// restrictions of the Go typesystem, but callers are not permitted to mutate
// any part of the returned data structures.
type Dir struct {
	baseDir        string
	targetPlatform getproviders.Platform

	// metaCache is a cache of the metadata of relevant packages available in
	// the cache directory last time we scanned it. This can be nil to indicate
	// that the cache is cold. The cache will be invalidated (set back to nil)
	// by any operation that modifies the contents of the cache directory.
	//
	// We intentionally don't make effort to detect modifications to the
	// directory made by other codepaths because the contract for NewDir
	// explicitly defines using the same directory for multiple purposes
	// as undefined behavior.
	// However, this code is now used for the global provider cache. With
	// the added support for locking, the data may no longer be valid with
	// changes from other processes. In practice this means that some packages
	// may have been installed since the latest re-scan. The code that
	// handles the installation should be smart enough to detect that and
	// work around it.
	metaCache map[addrs.Provider][]CachedProvider
}

// NewDir creates and returns a new Dir object that will read and write
// provider plugins in the given filesystem directory.
//
// If two instances of Dir are concurrently operating on a particular base
// directory, or if a Dir base directory is also used as a filesystem mirror
// source directory, the behavior is undefined.
func NewDir(baseDir string) *Dir {
	return &Dir{
		baseDir:        baseDir,
		targetPlatform: getproviders.CurrentPlatform,
	}
}

// NewDirWithPlatform is a variant of NewDir that allows selecting a specific
// target platform, rather than taking the current one where this code is
// running.
//
// This is primarily intended for portable unit testing and not particularly
// useful in "real" callers.
func NewDirWithPlatform(baseDir string, platform getproviders.Platform) *Dir {
	return &Dir{
		baseDir:        baseDir,
		targetPlatform: platform,
	}
}

// BasePath returns the filesystem path of the base directory of this
// cache directory.
func (d *Dir) BasePath() string {
	return filepath.Clean(d.baseDir)
}

func (d *Dir) Lock(ctx context.Context, provider addrs.Provider, version getproviders.Version) (func() error, error) {
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

	var f *os.File

	// Try to create the lock file, wait up to 1 second for transient errors to clear.
	for timeout := time.After(time.Second); ; {
		var err error

		// Try to get a handle to the file (or create if it does not exist)
		// They will all end up with the same file handle on any correctly implemented filesystem.
		// This is one of the many reasons we recommend users look at the flock support of their
		// networked filesystems when using the global provider cache.
		// Windows: even though out flock creates an exclusive lock, we are still able to open a handle to this file and wait below for the actual lock to be provided.
		// Sometimes the creates can conflict and will need to be tried multiple times (incredibly uncommon).
		f, err = os.OpenFile(lockFile, os.O_RDWR|os.O_CREATE, 0644)
		if err == nil {
			// We don't defer f.Close() here as we explicitly want to handle it below
			break
		}

		select {
		case <-timeout:
			return nil, err
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			// Chill for a bit before trying again
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Wait for the file lock for up to 60s.  Might make sense to have the timeout be configurable for different network conditions / package sizes.
	for timeout := time.After(time.Second * 60); ; {
		// We have a valid file handle, let's try to lock it (nonblocking)
		err := flock.Lock(f)
		if err == nil {
			// Lock succeeded
			break
		}

		select {
		case <-timeout:
			if f != nil {
				f.Close()
			}
			return nil, fmt.Errorf("Unable to acquire file lock on %q: %w", lockFile, err)
		case <-ctx.Done():
			if f != nil {
				f.Close()
			}
			return nil, ctx.Err()
		default:
			// Chill for a bit before trying again
			time.Sleep(100 * time.Millisecond)
		}

	}

	log.Printf("[TRACE] Acquired global provider lock %s", lockFile)

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

// AllAvailablePackages returns a description of all of the packages already
// present in the directory. The cache entries are grouped by the provider
// they relate to and then sorted by version precedence, with highest
// precedence first.
//
// This function will return an empty result both when the directory is empty
// and when scanning the directory produces an error.
//
// The caller is forbidden from modifying the returned data structure in any
// way, even though the Go type system permits it.
func (d *Dir) AllAvailablePackages() map[addrs.Provider][]CachedProvider {
	if err := d.fillMetaCache(); err != nil {
		log.Printf("[WARN] Failed to scan provider cache directory %s: %s", d.baseDir, err)
		return nil
	}

	return d.metaCache
}

// ProviderVersion returns the cache entry for the requested provider version,
// or nil if the requested provider version isn't present in the cache.
func (d *Dir) ProviderVersion(provider addrs.Provider, version getproviders.Version) *CachedProvider {
	if err := d.fillMetaCache(); err != nil {
		return nil
	}

	for _, entry := range d.metaCache[provider] {
		// We're intentionally comparing exact version here, so if either
		// version number contains build metadata and they don't match then
		// this will not return true. The rule of ignoring build metadata
		// applies only for handling version _constraints_ and for deciding
		// version precedence.
		if entry.Version == version {
			return &entry
		}
	}

	return nil
}

// ProviderLatestVersion returns the cache entry for the latest
// version of the requested provider already available in the cache, or nil if
// there are no versions of that provider available.
func (d *Dir) ProviderLatestVersion(provider addrs.Provider) *CachedProvider {
	if err := d.fillMetaCache(); err != nil {
		return nil
	}

	entries := d.metaCache[provider]
	if len(entries) == 0 {
		return nil
	}

	return &entries[0]
}

func (d *Dir) fillMetaCache() error {
	// For d.metaCache we consider nil to be different than a non-nil empty
	// map, so we can distinguish between having scanned and got an empty
	// result vs. not having scanned successfully at all yet.
	if d.metaCache != nil {
		log.Printf("[TRACE] providercache.fillMetaCache: using cached result from previous scan of %s", d.baseDir)
		return nil
	}
	log.Printf("[TRACE] providercache.fillMetaCache: scanning directory %s", d.baseDir)

	allData, err := getproviders.SearchLocalDirectory(d.baseDir)
	if err != nil {
		log.Printf("[TRACE] providercache.fillMetaCache: error while scanning directory %s: %s", d.baseDir, err)
		return err
	}

	// The getproviders package just returns everything it found, but we're
	// interested only in a subset of the results:
	// - those that are for the current platform
	// - those that are in the "unpacked" form, ready to execute
	// ...so we'll filter in these ways while we're constructing our final
	// map to save as the cache.
	//
	// We intentionally always make a non-nil map, even if it might ultimately
	// be empty, because we use that to recognize that the cache is populated.
	data := make(map[addrs.Provider][]CachedProvider)

	for providerAddr, metas := range allData {
		for _, meta := range metas {
			if meta.TargetPlatform != d.targetPlatform {
				log.Printf("[TRACE] providercache.fillMetaCache: ignoring %s because it is for %s, not %s", meta.Location, meta.TargetPlatform, d.targetPlatform)
				continue
			}
			if _, ok := meta.Location.(getproviders.PackageLocalDir); !ok {
				// PackageLocalDir indicates an unpacked provider package ready
				// to execute.
				log.Printf("[TRACE] providercache.fillMetaCache: ignoring %s because it is not an unpacked directory", meta.Location)
				continue
			}

			packageDir := filepath.Clean(string(meta.Location.(getproviders.PackageLocalDir)))

			log.Printf("[TRACE] providercache.fillMetaCache: including %s as a candidate package for %s %s", meta.Location, providerAddr, meta.Version)
			data[providerAddr] = append(data[providerAddr], CachedProvider{
				Provider:   providerAddr,
				Version:    meta.Version,
				PackageDir: filepath.ToSlash(packageDir),
			})
		}
	}

	// After we've built our lists per provider, we'll also sort them by
	// version precedence so that the newest available version is always at
	// index zero. If there are two versions that differ only in build metadata
	// then it's undefined but deterministic which one we will select here,
	// because we're preserving the order returned by SearchLocalDirectory
	// in that case..
	for _, entries := range data {
		sort.SliceStable(entries, func(i, j int) bool {
			// We're using GreaterThan rather than LessThan here because we
			// want these in _decreasing_ order of precedence.
			return entries[i].Version.GreaterThan(entries[j].Version)
		})
	}

	d.metaCache = data
	return nil
}
