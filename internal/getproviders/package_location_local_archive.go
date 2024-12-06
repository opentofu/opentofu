// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-getter"
)

// We borrow the "unpack a zip file into a target directory" logic from
// go-getter, even though we're not otherwise using go-getter here.
// (We don't need the same flexibility as we have for modules, because
// providers _always_ come from provider registries, which have a very
// specific protocol and set of expectations.)
//
//nolint:gochecknoglobals // this variable predates our use of this linter
var unzip = getter.ZipDecompressor{}

// PackageLocalArchive is the location of a provider distribution archive file
// in the local filesystem. Its value is a local filesystem path using the
// syntax understood by Go's standard path/filepath package on the operating
// system where OpenTofu is running.
type PackageLocalArchive string

var _ PackageLocation = PackageLocalArchive("")

func (p PackageLocalArchive) String() string { return string(p) }

func (p PackageLocalArchive) InstallProviderPackage(_ context.Context, meta PackageMeta, targetDir string, allowedHashes []Hash) (*PackageAuthenticationResult, error) {
	var authResult *PackageAuthenticationResult
	if meta.Authentication != nil {
		var err error
		if authResult, err = meta.Authentication.AuthenticatePackage(meta.Location); err != nil {
			return nil, err
		}
	}

	if len(allowedHashes) > 0 {
		if matches, err := meta.MatchesAnyHash(allowedHashes); err != nil {
			return authResult, fmt.Errorf(
				"failed to calculate checksum for %s %s package at %s: %w",
				meta.Provider, meta.Version, meta.Location, err,
			)
		} else if !matches {
			return authResult, fmt.Errorf(
				"the current package for %s %s doesn't match any of the checksums previously recorded in the dependency lock file; for more information: https://opentofu.org/docs/language/files/dependency-lock/#checksum-verification",
				meta.Provider, meta.Version,
			)
		}
	}

	filename := meta.Location.String()

	// NOTE: We're not checking whether there's already a directory at
	// targetDir with some files in it. Packages are supposed to be immutable
	// and therefore we'll just be overwriting all of the existing files with
	// their same contents unless something unusual is happening. If something
	// unusual _is_ happening then this will produce something that doesn't
	// match the allowed hashes and so our caller should catch that after
	// we return if so.

	//nolint:mnd // magic number predates us using this linter
	err := unzip.Decompress(targetDir, filename, true, 0000)
	if err != nil {
		return authResult, err
	}

	return authResult, nil
}
