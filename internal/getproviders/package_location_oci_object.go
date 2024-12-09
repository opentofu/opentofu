// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/opentofu/libregistry/registryprotocols/ociclient"
)

// PackageOCIObject refers to an object in an OCI repository that is to be
// treated as a provider package.
//
// The manifest associated with the given digest should be a single
// image manifest for a specific platform. It should _not_ be a multi-platform
// manifest, because the decision about which platform to select should
// have already been made by whatever generates an object of this type.
type PackageOCIObject struct {
	repositoryAddr      OCIRepository
	imageManifestDigest ociclient.OCIDigest

	// client is the OCI client that should be used to retrieve the
	// object's layers.
	client ociclient.OCIClient
}

var _ PackageLocation = PackageOCIObject{}

func (p PackageOCIObject) String() string {
	return fmt.Sprintf("%s@%s", p.repositoryAddr, p.imageManifestDigest)
}

func (p PackageOCIObject) InstallProviderPackage(ctx context.Context, meta PackageMeta, targetDir string, allowedHashes []Hash) (*PackageAuthenticationResult, error) {
	// If a previous run was able to capture a set of signed manifest digests
	// in allowedHashes then we can potentially reject a not-previously-known
	// manifest immediately before we fetch anything.
	err := p.checkHashesPreinstall(meta, allowedHashes)
	if err != nil {
		return nil, err
	}

	// FIXME: This API cannot currently return warnings, so we just discard them.
	var files ociclient.PulledOCIImage
	files, _, err = p.client.PullImageWithImageDigest(ctx, ociclient.OCIAddrWithDigest{
		OCIAddr: p.repositoryAddr.toClient(),
		Digest:  p.imageManifestDigest,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to pull OCI object %s: %w", p.String(), err)
	}
	defer files.Close()

	// If there's already a directory present at targetDir then we'll delete it first
	// because otherwise we'll end up merging the new package content with whatever
	// was already there, which would cause confusing checksum mismatches.
	err = os.RemoveAll(targetDir)
	if err != nil {
		return nil, fmt.Errorf("failed to remove existing provider package cache directory %s before installing a new copy: %w", targetDir, err)
	}

	// Since we'll be assembling the package directory gradually as we iterate over
	// the directory entries in the layers, we'll need to make the containing directory
	// separately first.
	const modeUserWritableOtherReadExecutable = 0755
	err = os.MkdirAll(targetDir, modeUserWritableOtherReadExecutable)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider package cache directory %s: %w", targetDir, err)
	}

	// Installing this OCI object will cause the package to become available in a
	// local directory too, and we'll be using that for verifying checksums and
	// any package authentication.
	localLoc := PackageLocalDir(targetDir)

	for {
		var haveNext bool
		haveNext, err = files.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to pull next file for OCI object %s: %w", p.String(), err)
		}
		if !haveNext {
			break // no more files
		}

		info := files.FileInfo()
		targetFilename := filepath.Join(targetDir, info.Name())
		if !filepath.IsLocal(targetFilename) {
			return nil, fmt.Errorf("layer for %s contains invalid file path %q", p.String(), targetFilename)
		}
		// FIXME: Need to also check that we're not writing through a symlink, and
		// possibly other hazards.
		// Ideally we'd use the result of this proposal: https://github.com/golang/go/issues/67002
		//
		// FIXME: Must also check that info.Mode.Perm is something reasonable. We
		// only really need to support the subset of modes that git supports:
		// non-executable regular file, executable regular file, directory, symlink.

		if info.IsDir() {
			err = os.Mkdir(targetFilename, info.Mode().Perm())
			if err != nil {
				return nil, fmt.Errorf("while extracting OCI object %s: %w", p.String(), err)
			}
		} else {
			// TODO: What if it's a symlink?

			var f *os.File
			f, err = os.OpenFile(targetFilename, os.O_CREATE|os.O_TRUNC|os.O_RDWR, info.Mode().Perm())
			if err != nil {
				return nil, fmt.Errorf("while extracting OCI object %s: %w", p.String(), err)
			}
			defer f.Close()

			_, err = io.Copy(f, files)
			if err != nil {
				return nil, fmt.Errorf("while extracting OCI object %s: %w", p.String(), err)
			}
		}

		// TODO: Is it important to retain any other metadata from the entries, such as
		// their last modified times? Our checksum algorithm doesn't care about it
		// but maybe something else will.
	}

	if len(allowedHashes) > 0 {
		var matches bool
		if matches, err = PackageMatchesAnyHash(localLoc, allowedHashes); err != nil {
			return nil, fmt.Errorf(
				"failed to calculate checksum for %s %s package at %s: %w",
				meta.Provider, meta.Version, meta.Location, err,
			)
		} else if !matches {
			return nil, fmt.Errorf(
				"the local package for %s %s in %s doesn't match any of the checksums previously recorded in the dependency lock file (this might be because the available checksums are for packages targeting different platforms); for more information: https://opentofu.org/docs/language/files/dependency-lock/#checksum-verification",
				meta.Provider, meta.Version, localLoc,
			)
		}
	}

	if meta.Authentication != nil {
		return meta.Authentication.AuthenticatePackage(localLoc)
	}
	//nolint:nilnil // this API predates our use of this linter and callers rely on this behavior
	return nil, nil
}

func (p PackageOCIObject) checkHashesPreinstall(meta PackageMeta, allowedHashes []Hash) error {
	// If any of the allowedHashes are of schemes that we can apply to a
	// not-yet-fetched OCI object manifest then we'll pre-verify them here.
	// We permit installation in the absense of any relevant hashes because
	// in practice we can only save OCI object manifest hashes in the
	// dependency lock file when the previous run installed from a signed
	// multi-platform manifest, and so this is only some extra assurance
	// for that case to catch when a previously-signed image got modified.
	suitableHashCount := 0
	for _, hash := range allowedHashes {
		if hash.Scheme().SupportsLocation(p) {
			suitableHashCount++
		}
	}
	if suitableHashCount == 0 {
		// The previous run presumably didn't involve a signed manifest
		// and so we'll rely on the post-install verification of h1:
		// hashes against the local cache directory later in
		// [PackageOCIObject.InstallProviderPackage] instead.
		return nil
	}

	if matches, err := PackageMatchesAnyHash(p, allowedHashes); err != nil {
		return fmt.Errorf(
			"failed to calculate checksum for %s %s package at %s: %w",
			meta.Provider, meta.Version, p, err,
		)
	} else if !matches {
		return fmt.Errorf(
			"the package for %s %s at %s doesn't match any of the checksums previously recorded in the dependency lock file (this might be because the available checksums are for packages targeting different platforms); for more information: https://opentofu.org/docs/language/files/dependency-lock/#checksum-verification",
			meta.Provider, meta.Version, p,
		)
	}

	return nil
}
