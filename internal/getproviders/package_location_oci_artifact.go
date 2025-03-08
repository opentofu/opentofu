package getproviders

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/hashicorp/go-getter"
	ociDigest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	orasContent "oras.land/oras-go/v2/content"
)

// Our manifest layout for providers uses specific artifactType values for each of
// its components, consistent with the typical recommendations for using OCI
// Distribution for artifact distribution, so that we can distinguish the specific
// entries that are intended for current OpenTofu from entries that might be present
// for entirely unrelated purposes, or entries that might be relevant to future
// OpenTofu versions with additional features we've not designed yet.
//
// Our implementation must therefore silently ignore any descriptors that are not
// of the expected artifact types.
const ociIndexManifestArtifactType = "application/vnd.opentofu.provider"
const ociPackageManifestArtifactType = "application/vnd.opentofu.provider-target"
const ociPackageArtifactType = "application/vnd.opentofu.providerpkg"

// ociPackageMediaType is the specific media type we're expecting for the blob
// representing a final distribution package that we'll fetch and extract, after
// we've dug through all of the manifests.
//
// We currently pay attention only to blobs which have both this media type AND
// the artifact type in ociPackageArtifactType, silently ignoring everything else,
// so that future versions of OpenTofu can potentially support provider packages
// using different archive formats.
const ociPackageMediaType = "archive/zip"

// ociImageManifestSizeLimit is the maximum size of artifact manifest (aka "image
// manifest") we'll accept. This 4MiB value matches the recommended limit for
// repositories to accept on push from the OCI Distribution v1.1 spec.
const ociImageManifestSizeLimitMiB = 4

// PackageOCIArtifact represents the digest of a platform-specific artifact manifest
// (NOT a multi-platform index manifest) stored in a specific OCI Distribution
// repository.
//
// A [Source] returning this kind of location must choose the appropriate single
// artifact from a provider's multi-platform index manifest and describe the
// location of that, so that the decision about which platform to select does not
// need to be repeated when fetching the package.
type PackageOCIArtifact struct {
	// Unlike the other PackageLocation types, this one has its internals unexported
	// so that we can make use of ORAS-Go types as an implementation detail without
	// exposing that decision in this package's public API.

	// repoStore is the ORAS storage that the artifact should be fetched from.
	//
	// In normal use this will typically be an instance of ORAS-Go's remote.Repository
	// type representing a specific remote repository, but it's also valid to use
	// a local filesystem or in-memory representation e.g. in unit tests.
	repoStore orasContent.ReadOnlyStorage

	// registryDomain and repositoryName are UI-display-only values together describing
	// which repository the repoStore object is configured to access, so that we can
	// describe what we're downloading in a human-friendly way.
	//
	// registryDomain is empty if repoStore is not for a remote repository. In that case
	// repositoryName might be a filesystem path if using a local OCI layout directory/archive,
	// or also empty if it's an in-memory-only store as we use in some tests.
	registryDomain, repositoryName string

	// manifestDescriptor is the in-memory representation of an OCI artifact descriptor
	// for the manifest of the OCI artifact this object represents.
	//
	// The MediaType, ArtifactType, and Platform fields must be consistent with the
	// expectations of a single-platform artifact manifest representing a provider
	// package or [PackageOCIArtifact.InstallProviderPackage] will fail with an error.
	manifestDescriptor ociv1.Descriptor
}

var _ PackageLocation = PackageOCIArtifact{}

func (p PackageOCIArtifact) InstallProviderPackage(ctx context.Context, meta PackageMeta, targetDir string, allowedHashes []Hash) (*PackageAuthenticationResult, error) {
	// First we'll make sure that what we've been given makes sense to be a single-platform
	// artifact manifest for the given package. A failure here suggests a bug in the
	// [Source] that generated this location object, so these checks are here just for
	// robustness and to help future maintainers understand how these different components
	// are intended to work together. Overall what we're expecting here is a verbatim
	// copy of the single selected manifest descriptor taken from the multi-platform
	// index manifest for this provider version.
	err := checkOCIDescriptorMatchesPackageMeta(p.manifestDescriptor, meta)
	if err != nil {
		return nil, err
	}

	// With all of the above checked, we should now be able to retrieve the manifest
	// to find the digest of the specific .zip archive blob we're going to fetch.
	manifest, err := fetchOCIArtifactManifest(ctx, p.manifestDescriptor, p.repoStore)
	if err != nil {
		return nil, fmt.Errorf("fetching manifest from %s: %w", p.String(), err)
	}

	pkgDesc, err := selectProviderPackageDescriptor(manifest)
	if err != nil {
		return nil, fmt.Errorf("unsupported OCI artifact at %s: %w", p.String(), err)
	}

	// If we have a fixed set of allowed hashes then we'll check that our
	// selected descriptor matches before we waste time fetching the package.
	if len(allowedHashes) > 0 && !ociPackageDescriptorDigestMatchesAnyHash(pkgDesc.Digest, allowedHashes) {
		return nil, fmt.Errorf(
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

func (p PackageOCIArtifact) String() string {
	switch {
	case p.registryDomain != "" && p.repositoryName != "":
		// This is the main case for real end-user use, where we should
		// always be talking to a repository in a remote registry.
		return fmt.Sprintf(
			"%s/%s@%s",
			p.registryDomain,
			p.repositoryName,
			p.manifestDescriptor.Digest.String(),
		)
	case p.repositoryName != "":
		// A local-filesystem-based repository is not currently something
		// we support as an end-user-facing feature, so this situation is
		// currently just for testing purposes.
		return fmt.Sprintf(
			"./%s@%s", // ./ prefix just to distinguish this from a remote registry address
			filepath.Clean(p.repositoryName),
			p.manifestDescriptor.Digest.String(),
		)
	default:
		// If we don't have either of the human-oriented location fields
		// populated then we'll just use the digest, and hope that the
		// reader knows what store we're using. This situation should
		// arise only in unit tests.
		return p.manifestDescriptor.Digest.String()
	}
}

func fetchOCIArtifactManifest(ctx context.Context, desc ociv1.Descriptor, store orasContent.Fetcher) (*ociv1.Manifest, error) {
	// We impose a size limit on the manifest just to avoid an abusive remote registry
	// occupuing unbounded memory when we read the manifest content into memory below.
	if (desc.Size / 1024 / 1024) > ociImageManifestSizeLimitMiB {
		return nil, fmt.Errorf("manifest size exceeds OpenTofu's size limit of %d MiB", ociImageManifestSizeLimitMiB)
	}

	readCloser, err := store.Fetch(ctx, desc)
	if err != nil {
		return nil, err
	}
	defer readCloser.Close()
	manifestReader := io.LimitReader(readCloser, desc.Size)

	// We need to verify that the content matches the digest in the descriptor,
	// and we also need to parse that data as JSON. We can only read from the
	// reader once, so we have no choice but to buffer it all in memory.
	manifestSrc, err := io.ReadAll(manifestReader)
	if err != nil {
		return nil, fmt.Errorf("reading manifest content: %w", err)
	}

	gotDigest := desc.Digest.Algorithm().FromBytes(manifestSrc)
	if gotDigest != desc.Digest {
		return nil, fmt.Errorf("manifest content does not match digest %s", desc.Digest)
	}

	var manifest ociv1.Manifest
	err = json.Unmarshal(manifestSrc, &manifest)
	if err != nil {
		return nil, fmt.Errorf("invalid manifest content: %w", err)
	}

	// Now we'll make sure that what we decoded seems vaguely sensible before we
	// return it.
	if manifest.Versioned.SchemaVersion != 2 {
		return nil, fmt.Errorf("unsupported manifest schema version %d", manifest.Versioned.SchemaVersion)
	}
	if manifest.MediaType != desc.MediaType {
		return nil, fmt.Errorf("unexpected manifest media type %q", manifest.MediaType)
	}
	if manifest.ArtifactType != desc.ArtifactType {
		return nil, fmt.Errorf("unexpected artifact type %q", manifest.ArtifactType)
	}
	// We intentionally leave everything else loose so that we'll have flexibility
	// to extend this format in backward-compatible ways in future OpenTofu versions.
	return &manifest, nil
}

func selectProviderPackageDescriptor(manifest *ociv1.Manifest) (ociv1.Descriptor, error) {
	var haveWrongMediaType string
	var ret ociv1.Descriptor
	for _, candidate := range manifest.Layers {
		// We intentionally ignore any "layers" that don't have both the expected
		// artifact type _and_ media type because in future OpenTofu versions we
		// might want to support other archive formats in a backward-compatible
		// way by recognizing additional media types in association with our
		// artifact type.
		if candidate.ArtifactType == ociPackageArtifactType {
			if candidate.MediaType == ociPackageMediaType {
				if ret.ArtifactType != "" {
					// We've already found a suitable descriptor on a previous
					// iteration, so this manifest is ambiguous.
					return ret, fmt.Errorf("manifest specifies more than one provider package")
				}
				ret = candidate
				continue
			}
			haveWrongMediaType = candidate.MediaType
		}
	}
	if ret.ArtifactType != "" {
		// We've found a suitable descriptor to return, then.
		return ret, nil
	}

	// If we get here then we didn't find anything suitable, so we need to describe the
	// situation in an error message.
	if haveWrongMediaType != "" {
		// If we found something with the correct artifact type but an unexpected
		// media type then the most likely explanation is that a future version
		// of OpenTofu introduced a new supported archive format and whoever
		// packaged this provider has decided to phase out support for this
		// version of OpenTofu, so we'll hint that in the error message.
		// (If the manifest includes _multiple_ unsupported media types then
		// this will arbitrarily report just the last one for simplicity's sake.)
		return ret, fmt.Errorf("provider package has unsupported media type %q; is this package intended for a different OpenTofu version?", haveWrongMediaType)
	}
	return ret, fmt.Errorf("no suitable provider package declared in artifact manifest")
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

	f, err := os.CreateTemp("", "terraform-provider")
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
		os.Remove(f.Name()) // best effort to clean up our temporary file if we failed
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
	for _, candidate := range allowed {
		if foundHash == candidate {
			return true
		}
	}
	return false
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

func checkOCIDescriptorMatchesPackageMeta(desc ociv1.Descriptor, meta PackageMeta) error {
	if desc.ArtifactType != ociPackageManifestArtifactType {
		return fmt.Errorf("selected OCI artifact has unexpected type %q", desc.ArtifactType)
	}
	if desc.MediaType != ociv1.MediaTypeImageManifest {
		return fmt.Errorf("selected OCI artifact manifest has unexpected media type %q", desc.MediaType)
	}
	if desc.Platform == nil {
		return fmt.Errorf("artifact descriptor does not include target platform information")
	}
	if desc.Platform.OS != meta.TargetPlatform.OS || desc.Platform.Architecture != meta.TargetPlatform.Arch {
		// We'll use our own type here to produce an OpenTofu-conventional string representation
		gotPlatform := Platform{OS: desc.Platform.OS, Arch: desc.Platform.Architecture}
		return fmt.Errorf("selected OCI artifact is for %s, but was expected to be for %s", gotPlatform.String(), meta.TargetPlatform.String())
	}
	return nil
}
