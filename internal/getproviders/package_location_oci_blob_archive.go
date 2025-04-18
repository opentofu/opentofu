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
	"slices"

	"github.com/hashicorp/go-getter"
	ociDigest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	orasContent "oras.land/oras-go/v2/content"
)

// ociPackageMediaType is the specific media type we're expecting for the blob
// representing a final distribution package that we'll fetch and extract, after
// we've dug through all of the manifests.
//
// We currently pay attention only to blobs which have both this media type AND
// the artifact type in [ociPackageArtifactType], silently ignoring everything else,
// so that future versions of OpenTofu can potentially support provider packages
// using different archive formats.
const ociPackageMediaType = "archive/zip"

// PackageOCIBlobArchive represents a provider package archive stored as a blob in
// an OCI Distribution repository.
//
// A [Source] returning this kind of location must first choose the appropriate
// artifact from a provider's multi-platform index manifest, fetch that manifest,
// and identify the single leaf blob whose content is the provider package
// archive. This type does not interact with artifact manifests at all, expecting
// that whatever constructed it has already interrogated all of the relevant
// manifests.
type PackageOCIBlobArchive struct {
	// Unlike the other PackageLocation types, this one has its internals unexported
	// so that we can make use of ORAS-Go types as an implementation detail without
	// exposing that decision in this package's public API.

	// repoStore is the ORAS storage that the artifact should be fetched from.
	//
	// In normal use this will typically be an instance of ORAS-Go's remote.Repository
	// type representing a specific remote repository, but it's also valid to use
	// a local filesystem or in-memory representation e.g. in unit tests.
	repoStore orasContent.Fetcher

	// registryDomain and repositoryName are UI-display-only values together describing
	// which repository the repoStore object is configured to access, so that we can
	// describe what we're downloading in a human-friendly way.
	//
	// registryDomain is empty if repoStore is not for a remote repository. In that case
	// repositoryName might be a filesystem path if using a local OCI layout directory/archive,
	// or also empty if it's an in-memory-only store as we use in some tests.
	registryDomain, repositoryName string

	// blobDescriptor is the in-memory representation of an OCI descriptor
	// for the leaf blob to retrieve.
	//
	// We currently require that the digest in the descriptor use the "sha256"
	// algorithm in particular, because that is directly analogous to
	// our [HashSchemeZip] and thus we can cross-verify the archive against
	// any signed checksums provided by the provider author from the provider's
	// origin registry that were recorded into the dependency lock file after
	// an earlier install from the origin registry.
	//
	// The MediaType and ArtifactType fields must represent valid selections for a
	// provider package stored in an OCI Distribution repository. Currently that
	// means that MediaType must be "archive/zip" and ArtifactType must be
	// "application/vnd.opentofu.providerpkg"; future OpenTofu formats might support
	// other file formats, which can be represented by choosing a new value of
	// MediaType while retaining the same ArtifactType.
	blobDescriptor ociv1.Descriptor
}

var _ PackageLocation = PackageOCIBlobArchive{}

func (p PackageOCIBlobArchive) InstallProviderPackage(ctx context.Context, meta PackageMeta, targetDir string, allowedHashes []Hash) (*PackageAuthenticationResult, error) {
	pkgDesc := p.blobDescriptor

	// First we'll make sure that what we've been given makes sense to be the descriptor
	// for a provider package blob. A failure here suggests a bug in the [Source] that
	// generated this location object, since it should not select a blob that this
	// location type cannot support.
	err := checkOCIBlobDescriptor(pkgDesc, meta)
	if err != nil {
		return nil, err
	}

	// If we have a fixed set of allowed hashes then we'll check that our
	// selected descriptor matches before we waste time fetching the package.
	if len(allowedHashes) > 0 && !ociPackageDescriptorDigestMatchesAnyHash(pkgDesc.Digest, allowedHashes) {
		return nil, fmt.Errorf(
			// FIXME: We currently have slightly-different variations of this error
			// message spread across the different [PackageLocation] implementations.
			// It would be good to settle on a single good version of this text,
			// centralize it into a constant or function somewhere, and then reuse
			// it across all of the package location types. For now though, this
			// matches some of the others so we can treat that refactoring as a
			// separate task from implementing OCI-based installation as a new feature.
			"the current package for %s %s doesn't match any of the checksums previously recorded in the dependency lock file; for more information: https://opentofu.org/docs/language/files/dependency-lock/#checksum-verification",
			meta.Provider, meta.Version,
		)
	}

	// If we reach this point then we have a descriptor for what will hopefully
	// turn out to be a valid provider package blob, and so we'll download it
	// into a temporary file from which we can verify it and then extract it
	// into its final location.
	localLoc, err := fetchOCIBlobToTemporaryFile(ctx, pkgDesc, p.repoStore)
	if err != nil {
		return nil, fmt.Errorf("fetching provider package blob %s: %w", pkgDesc.Digest.String(), err)
	}
	defer os.Remove(string(localLoc)) // Best effort to remove the temporary file before we return

	// We'll now delegate the final installation step to the localLoc object,
	// which knows how to extract the temporary archive into the target
	// directory and verify/authenticate it.
	// To do that we need a slightly-modified "meta" that refers to the
	// local location instead of the remote one. (This redundancy is awkward,
	// but is a historical artifact of how package installation used to be
	// separated from the PackageLocation type. We're accepting that awkwardness
	// for now to avoid a risky refactor, but hopefully we'll tidy this up
	// someday.)
	localMeta := PackageMeta{
		Provider:         meta.Provider,
		Version:          meta.Version,
		ProtocolVersions: meta.ProtocolVersions,
		TargetPlatform:   meta.TargetPlatform,
		Filename:         meta.Filename,
		Location:         localLoc,
		Authentication:   meta.Authentication,
	}
	return localLoc.InstallProviderPackage(ctx, localMeta, targetDir, allowedHashes)
}

func (p PackageOCIBlobArchive) String() string {
	switch {
	case p.registryDomain != "" && p.repositoryName != "":
		// This is the main case for real end-user use, where we should
		// always be talking to a repository in a remote registry.
		return fmt.Sprintf(
			"%s/%s@%s",
			p.registryDomain,
			p.repositoryName,
			p.blobDescriptor.Digest.String(),
		)
	case p.repositoryName != "":
		// A local-filesystem-based repository is not currently something
		// we support as an end-user-facing feature, so this situation is
		// currently just for testing purposes.
		return fmt.Sprintf(
			"./%s@%s", // ./ prefix just to distinguish this from a remote registry address
			filepath.Clean(p.repositoryName),
			p.blobDescriptor.Digest.String(),
		)
	default:
		// If we don't have either of the human-oriented location fields
		// populated then we'll just use the digest, and hope that the
		// reader knows what store we're using. This situation should
		// arise only in unit tests.
		return p.blobDescriptor.Digest.String()
	}
}

// fetchOCIBlobToTemporaryFile uses the given ORAS fetcher to pull the content of the
// blob described by "desc" into a temporary file on the local filesystem, and
// then returns the path to that file as a [PackageLocalArchive] that can be used
// to delegate the final verification and installation of that archive into a
// target directory.
//
// It is the caller's responsibility to delete the temporary file once it's no longer
// needed.
func fetchOCIBlobToTemporaryFile(ctx context.Context, desc ociv1.Descriptor, store orasContent.Fetcher) (loc PackageLocalArchive, err error) {
	// This is effectively an OCI Distribution equivalent of the similar technique
	// used for PackageHTTPArchive.

	// We'll eventually need to generate an OpenTofu-style hash for this package anyway,
	// so we'll do that now to make sure we have a valid digest before we try to
	// download anything.
	wantHash, err := hashFromOCIDigest(desc.Digest)
	if err != nil {
		return PackageLocalArchive(""), fmt.Errorf("cannot verify package contents: %w", err)
	}

	readCloser, err := store.Fetch(ctx, desc)
	if err != nil {
		return PackageLocalArchive(""), err
	}
	defer readCloser.Close()
	reader := io.LimitReader(readCloser, desc.Size)

	f, err := os.CreateTemp("", "opentofu-provider")
	if err != nil {
		return PackageLocalArchive(""), fmt.Errorf("failed to open temporary file: %w", err)
	}
	loc = PackageLocalArchive(f.Name())
	defer func() {
		// If we're returning an error then the caller won't make use of the
		// file we've created, so we'll make a best effort to proactively
		// remove it. If we succeed then it's the caller's responsibility to
		// remove the file once it's no longer needed.
		if err != nil {
			os.Remove(f.Name())
		}
	}()

	// We'll borrow go-getter's "cancelable copy" implementation here so that
	// the download can potentially be interrupted partway through.
	n, err := getter.Copy(ctx, f, reader)
	f.Close() // we're done using the filehandle now, even if the copy failed
	if err == nil && n < desc.Size {
		// This should be impossible because we used io.LimitReader, but we'll check
		// anyway to be robust since go-getter returns this information regardless.
		err = fmt.Errorf("incorrect response size: expected %d bytes, but got %d bytes", desc.Size, n)
	}
	if err != nil {
		return loc, err
	}

	// Before we return we'll make sure that the file we've just created matches
	// the digest we were given for it, since the ORAS fetcher doesn't do that
	// automatically itself.
	matchesHash, err := PackageMatchesHash(loc, wantHash)
	if err != nil {
		return loc, fmt.Errorf("cannot verify package contents: %w", err)
	}
	if !matchesHash {
		return loc, fmt.Errorf("provider package does not match digest %s", desc.Digest)
	}

	// Everything seems okay, so we'll let the caller take ownership of the temporary file.
	return loc, nil
}

func ociPackageDescriptorDigestMatchesAnyHash(found ociDigest.Digest, allowed []Hash) bool {
	foundHash, err := hashFromOCIDigest(found)
	if err != nil {
		// An unsupported or invalid digest cannot possibly match
		return false
	}
	return slices.Contains(allowed, foundHash)
}

func hashFromOCIDigest(digest ociDigest.Digest) (Hash, error) {
	if err := digest.Validate(); err != nil {
		return Hash(""), fmt.Errorf("invalid digest %q: %w", digest.String(), err)
	}
	if algo := digest.Algorithm(); algo != ociDigest.SHA256 {
		// OpenTofu's "ziphash" format requires a SHA256 checksum in particular.
		return Hash(""), fmt.Errorf("unsupported digest algorithm %q", algo.String())
	}
	// If we have a valid sha256 digest then we can use its payload
	// directly as the payload for our "ziphash" checksum scheme,
	// because both we and OCI use lowercase hex encoding for these.
	return HashSchemeZip.New(digest.Encoded()), nil
}

func checkOCIBlobDescriptor(desc ociv1.Descriptor, meta PackageMeta) error {
	if desc.MediaType != ociPackageMediaType {
		return fmt.Errorf("selected OCI artifact manifest has unexpected media type %q", desc.MediaType)
	}
	if desc.Platform != nil {
		// A descriptor can optionally by annotated by a platform selection. We don't
		// require this because the top-level index manifest for a provider artifact
		// is enough of a signal of this information, but if someone has gone to the
		// trouble of annotating their blob descriptor with a platform then we'll
		// use that to generate a more useful error message than is likely to occur
		// if we later try to run an executable intended for a different platform.
		if desc.Platform.OS != meta.TargetPlatform.OS || desc.Platform.Architecture != meta.TargetPlatform.Arch {
			// We'll use our own type here to produce an OpenTofu-conventional string representation
			gotPlatform := Platform{OS: desc.Platform.OS, Arch: desc.Platform.Architecture}
			return fmt.Errorf("selected OCI artifact is for %s, but was expected to be for %s", gotPlatform.String(), meta.TargetPlatform.String())
		}
	}
	return nil
}
