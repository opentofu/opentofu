// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/opentofu/opentofu/internal/copy"
)

// PackageLocalDir is the location of a directory containing an unpacked
// provider distribution archive in the local filesystem. Its value is a local
// filesystem path using the syntax understood by Go's standard path/filepath
// package on the operating system where OpenTofu is running.
type PackageLocalDir string

var _ PackageLocation = PackageLocalDir("")

func (p PackageLocalDir) String() string { return string(p) }

func (p PackageLocalDir) InstallProviderPackage(_ context.Context, meta PackageMeta, targetDir string, allowedHashes []Hash) (*PackageAuthenticationResult, error) {
	sourceDir := meta.Location.String()

	absNew, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, fmt.Errorf("failed to make target path %s absolute: %w", targetDir, err)
	}
	absCurrent, err := filepath.Abs(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to make source path %s absolute: %w", sourceDir, err)
	}

	// Before we do anything else, we'll do a quick check to make sure that
	// these two paths are not pointing at the same physical directory on
	// disk. This compares the files by their OS-level device and directory
	// entry identifiers, not by their virtual filesystem paths.
	var same bool
	if same, err = copy.SameFile(absNew, absCurrent); same {
		return nil, fmt.Errorf("cannot install existing provider directory %s to itself", targetDir)
	} else if err != nil {
		return nil, fmt.Errorf("failed to determine if %s and %s are the same: %w", sourceDir, targetDir, err)
	}

	var authResult *PackageAuthenticationResult
	if meta.Authentication != nil {
		// (we have this here for completeness but note that local filesystem
		// mirrors typically don't include enough information for package
		// authentication and so we'll rarely get in here in practice.)
		if authResult, err = meta.Authentication.AuthenticatePackage(meta.Location); err != nil {
			return nil, err
		}
	}

	// If the caller provided at least one hash in allowedHashes then at
	// least one of those hashes ought to match. However, for local directories
	// in particular we can't actually verify the legacy "zh:" hash scheme
	// because it requires access to the original .zip archive, and so as a
	// measure of pragmatism we'll treat a set of hashes where all are "zh:"
	// the same as no hashes at all, and let anything pass. This is definitely
	// non-ideal but accepted for two reasons:
	// - Packages we find on local disk can be considered a little more trusted
	//   than packages coming from over the network, because we assume that
	//   they were either placed intentionally by an operator or they were
	//   automatically installed by a previous network operation that would've
	//   itself verified the hashes.
	// - Our installer makes a concerted effort to record at least one new-style
	//   hash for each lock entry, so we should very rarely end up in this
	//   situation anyway.
	suitableHashCount := 0
	for _, hash := range allowedHashes {
		if !hash.HasScheme(HashSchemeZip) {
			suitableHashCount++
		}
	}
	if suitableHashCount > 0 {
		var matches bool
		if matches, err = meta.MatchesAnyHash(allowedHashes); err != nil {
			return authResult, fmt.Errorf(
				"failed to calculate checksum for %s %s package at %s: %w",
				meta.Provider, meta.Version, meta.Location, err,
			)
		} else if !matches {
			return authResult, fmt.Errorf(
				"the local package for %s %s doesn't match any of the checksums previously recorded in the dependency lock file (this might be because the available checksums are for packages targeting different platforms); for more information: https://opentofu.org/docs/language/files/dependency-lock/#checksum-verification",
				meta.Provider, meta.Version,
			)
		}
	}

	// Delete anything that's already present at this path first.
	err = os.RemoveAll(targetDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to remove existing %s before linking it to %s: %w", sourceDir, targetDir, err)
	}

	// We'll prefer to create a symlink if possible, but we'll fall back to
	// a recursive copy if symlink creation fails. It could fail for a number
	// of reasons, including being on Windows 8 without administrator
	// privileges or being on a legacy filesystem like FAT that has no way
	// to represent a symlink. (Generalized symlink support for Windows was
	// introduced in a Windows 10 minor update.)
	//
	// We use an absolute path for the symlink to reduce the risk of it being
	// broken by moving things around later, since the source directory is
	// likely to be a shared directory independent on any particular target
	// and thus we can't assume that they will move around together.
	linkTarget := absCurrent

	parentDir := filepath.Dir(absNew)
	//nolint:mnd // magic number predates us using this linter
	err = os.MkdirAll(parentDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create parent directories leading to %s: %w", targetDir, err)
	}

	err = os.Symlink(linkTarget, absNew)
	if err == nil {
		// Success, then!
		//nolint:nilnil // this API predates our use of this linter and callers rely on this behavior
		return nil, nil
	}

	// If we get down here then symlinking failed and we need a deep copy
	// instead. To make a copy, we first need to create the target directory,
	// which would otherwise be a symlink.
	//nolint:mnd // magic number predates us using this linter
	err = os.Mkdir(absNew, 0755)
	if err != nil && os.IsExist(err) {
		return nil, fmt.Errorf("failed to create directory %s: %w", absNew, err)
	}
	err = copy.CopyDir(absNew, absCurrent)
	if err != nil {
		return nil, fmt.Errorf("failed to either symlink or copy %s to %s: %w", absCurrent, absNew, err)
	}

	// If we got here then apparently our copy succeeded, so we're done.
	//nolint:nilnil // this API predates our use of this linter and callers rely on this behavior
	return nil, nil
}
