// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package providercache

import (
	"context"
	"log"
	"os"
	"path/filepath"
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
	// handles the installation is smart enough to detect that now and
	// work around it.

	testCache map[addrs.Provider][]CachedProvider
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
	lockFile := filepath.Join(providerPath, ".lock")

	log.Printf("[TRACE] Attempting to acquire global provider lock %s", lockFile)

	// Ensure the provider directory exists
	//nolint: mnd // directory permissions
	if err := os.MkdirAll(providerPath, 0755); err != nil {
		return nil, err
	}

	var err error
	var f *os.File

	// Try to create the lock file, wait up to 1 second for transient errors to clear.
	for start := time.Now(); time.Since(start) < time.Second*1; {
		// Check if the context is still active
		err = ctx.Err()
		if err != nil {
			break
		}

		// Try to get a handle to the file (or create if it does not exist)
		// Sometimes the creates can conflict and will need to be tried multiple times.
		//nolint: mnd // file permissions
		f, err = os.OpenFile(lockFile, os.O_RDWR|os.O_CREATE, 0644)
		if err == nil {
			// We don't defer f.Close() here as we explicitly want to handle
			break
		}
		//nolint: mnd // Chill for 50ms before trying again
		time.Sleep(50 * time.Millisecond)
	}

	if err != nil {
		return nil, err
	}

	// Wait for the file lock for up to 60s.  Might make sense to have the timeout be configurable for different network conditions / package sizes.
	for start := time.Now(); time.Since(start) < time.Second*60; {
		// Check if the context is still active
		err = ctx.Err()
		if err != nil {
			break
		}

		// We have a valid file handle, let's try to lock it (nonblocking)
		err = flock.Lock(f)
		if err == nil {
			// Lock succeeded
			break
		}
		//nolint: mnd // Chill for 100ms before trying again
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		// Early close the file
		if f != nil {
			f.Close()
		}

		return nil, err
	}

	log.Printf("[TRACE] Acquired global provider lock %s", lockFile)

	return func() error {
		log.Printf("[TRACE] Releasing global provider lock %s", lockFile)

		unlockErr := flock.Unlock(f)

		// Prefer close error over unlock error
		err = f.Close()
		if err != nil {
			return err
		}
		return unlockErr
	}, nil
}

// ProviderVersion returns the cache entry for the requested provider version,
// or nil if the requested provider version isn't present in the directory.
func (d *Dir) ProviderVersion(provider addrs.Provider, version getproviders.Version) *CachedProvider {
	fullPath := getproviders.UnpackedDirectoryPathForPackage(d.baseDir, provider, version, d.targetPlatform)

	stat, err := os.Stat(fullPath)
	if err != nil || !stat.IsDir() {
		log.Printf("[TRACE] No provider located at %s: %s", fullPath, err.Error())
		return nil
	}

	return &CachedProvider{
		Provider:   provider,
		Version:    version,
		PackageDir: filepath.ToSlash(fullPath),
	}

	for _, entry := range d.testCache[provider] {
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
