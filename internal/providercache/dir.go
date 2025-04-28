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

// ProviderVersion returns the cache entry for the requested provider version,
// or nil if the requested provider version isn't present in the cache.
func (d *Dir) ProviderVersion(provider addrs.Provider, version getproviders.Version) *CachedProvider {
	dir := getproviders.UnpackedDirectoryPathForPackage(d.baseDir, provider, version, d.targetPlatform)
	if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
		return &CachedProvider{
			Provider:   provider,
			Version:    version,
			PackageDir: dir,
		}
	}

	return nil
}
