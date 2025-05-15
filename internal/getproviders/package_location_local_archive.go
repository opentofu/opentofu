// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/hashicorp/go-getter"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"

	"github.com/opentofu/opentofu/internal/tracing"
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

func (p PackageLocalArchive) InstallProviderPackage(ctx context.Context, meta PackageMeta, targetDir string, allowedHashes []Hash) (*PackageAuthenticationResult, error) {
	_, span := tracing.Tracer().Start(ctx, "Decompress (local archive)")
	defer span.End()

	var authResult *PackageAuthenticationResult
	if meta.Authentication != nil {
		var err error
		if authResult, err = meta.Authentication.AuthenticatePackage(meta.Location); err != nil {
			return nil, err
		}
	}

	if len(allowedHashes) > 0 {
		if matches, err := meta.MatchesAnyHash(allowedHashes); err != nil {
			err := fmt.Errorf(
				"failed to calculate checksum for %s %s package at %s: %w",
				meta.Provider, meta.Version, meta.Location, err,
			)
			tracing.SetSpanError(span, err)
			return authResult, err
		} else if !matches {
			err := fmt.Errorf(
				"the current package for %s %s doesn't match any of the checksums previously recorded in the dependency lock file; for more information: https://opentofu.org/docs/language/files/dependency-lock/#checksum-verification",
				meta.Provider, meta.Version,
			)
			tracing.SetSpanError(span, err)
			return authResult, err
		}
	}

	filename := meta.Location.String()
	span.SetAttributes(semconv.FilePath(filename))

	// NOTE: Packages are immutable, but we may want to skip overwriting the existing
	// files in due to specific scenarios defined below.

	if _, err := os.Stat(targetDir); err == nil {
		// If the package might already be installed, we should try to skip overwriting the contents.
		// When run with TF_PLUGIN_CACHE_DIR or similar, a given provider might already be executing
		// and therefore locking the provider binary in the target directory (preventing the overwrite below)
		//
		// This does incur the overhead of two additional hash computations and could be
		// skipped with smarter checks around re-use scenarios in the future.

		targetHash, targetErr := PackageHashV1(PackageLocalDir(targetDir))
		fileHash, fileErr := PackageHashV1(meta.Location)

		if targetHash == fileHash && fileErr == nil && targetErr == nil {
			// Package is properly installed, bad or missing lock file will be caught elsewhere
			log.Printf("[INFO] Skipping local installation of provider %s %s as the existing contents already match the new contents", meta.Provider, meta.Version)
			return authResult, nil
		}
	}

	//nolint:mnd // magic number predates us using this linter
	err := unzip.Decompress(targetDir, filename, true, 0000)
	if err != nil {
		tracing.SetSpanError(span, err)
		return authResult, err
	}

	return authResult, nil
}
