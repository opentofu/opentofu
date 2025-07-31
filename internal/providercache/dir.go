// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package providercache

import (
	"os"
	"path/filepath"

	"github.com/opentofu/opentofu/internal/addrs"
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
