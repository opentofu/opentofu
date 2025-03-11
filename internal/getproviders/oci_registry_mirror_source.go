// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/apparentlymart/go-versions/versions"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	orasErrors "oras.land/oras-go/v2/errdef"

	"github.com/opentofu/opentofu/internal/addrs"
)

// ociIndexManifestArtifactType is the artifact type we expect for the top-level
// index manifest for an OpenTofu provider version.
//
// If a selected tag refers to a manifest that either lacks an artifact type
// or has a different artifact type then OpenTofu will reject it with an
// error indicating that it seems to be something other than an OpenTofu provider.
const ociIndexManifestArtifactType = "application/vnd.opentofu.provider"

// ociPackageManifestArtifactType is the artifact type we expect for each of the
// individual manifests listed in a provider version's top-level index manifest.
//
// OpenTofu will silently ignore any listed descriptors that either lack an artifact
// type or use a different one, both so that future versions of OpenTofu can
// be sensitive to additional artifact types if needed and so that those creating
// an artifact can choose to blend in other non-OpenTofu-related artifacts if
// they have some reason to do that.
//
// All descriptors with this artifact type MUST include a "platform" object
// with "os" and "architecture" set to match the platform that the individual
// package is built for. OpenTofu and OCI both use Go's codenames for operating
// systems and CPU architectures, so these fields should exactly match the
// names that would be used with this package's [Platform] type.
//
// OpenTofu does not currently make any use of the other properties defined for
// a "platform" object, and so will silently ignore any descriptors that set
// those properties. Future versions of OpenTofu might be able to use finer-grain
// platform selection properties, in which case those versions should treat
// a descriptor that uses those additional properties as higher precedence than
// one that doesn't so that manifest authors can include both a specific descriptor
// and a fallback descriptor with only os/architecture intended for use by
// earlier OpenTofu versions.
const ociPackageManifestArtifactType = "application/vnd.opentofu.provider-target"

// ociImageManifestSizeLimit is the maximum size of artifact manifest (aka "image
// manifest") we'll accept. This 4MiB value matches the recommended limit for
// repositories to accept on push from the OCI Distribution v1.1 spec:
//
//	https://github.com/opencontainers/distribution-spec/blob/v1.1.0/spec.md#pushing-manifests
const ociImageManifestSizeLimitMiB = 4

// OCIRegistryMirrorSource is a source that treats one or more repositories
// in a registry implementing the OCI Distribution protocol as a kind of
// provider mirror.
//
// This is conceptually similar to [HTTPMirrorSource], but whereas that one
// acts as a client for the OpenTofu-specific "provider mirror protocol"
// this one instead relies on a user-configured template to map provider
// source addresses onto OCI repository addresses and then uses tags in
// the selected registry to discover the available versions, and OCI
// manifests to represent their metadata.
//
// This implementation is currently intended to be compatible with
// OCI Distribution v1.1.0:
//
//	https://github.com/opencontainers/distribution-spec/blob/v1.1.0/spec.md
type OCIRegistryMirrorSource struct {
	// resolveOCIRepositoryAddr represents this source's rule for mapping
	// OpenTofu-style provider source addresses into OCI Distribution
	// repository addresses, which include both the domain name (and
	// optional port number) of the registry where the repository is
	// hosted and the name of the repository within that registry.
	//
	// This MUST behave as a pure function: a specific given provider
	// address must always return the same results, because it will
	// be called multiple times across the steps of selecting a provider
	// version.
	//
	// Implementers are responsible for somehow dealing with the fact
	// that OpenTofu-style provider source addresses support a
	// considerably wider set of Unicode characters than OCI Distribution
	// repository names do. That could mean either translating unsupported
	// characters into a different representation, or as a last resort
	// returning an error message explaining the problem in terms that make
	// sense for however the end user would've defined the mapping rule.
	//
	// If this function returns with a nil error then registryDomain
	// must be a valid domain name optionally followed by a colon
	// and a decimal port number, and repositoryName must be a string
	// conforming to the "<name>" pattern defined in the OCI Distribution
	// specification. The source will return low-quality error messages
	// if the results are unsuitable, and so implementers should prefer
	// to return their own higher-quality error diagnostics if no valid
	// mapping is possible.
	//
	// Any situation where the requested provider cannot be supported
	// _at all_ MUST return an instance of [ErrProviderNotFound] so
	// that a [MultiSource] can successfully blend the results from
	// this and other sources.
	resolveOCIRepositoryAddr func(ctx context.Context, addr addrs.Provider) (registryDomain, repositoryName string, err error)

	// getOCIRepositoryStore is the dependency inversion adapter for
	// obtaining a suitably-configured client for the given repository
	// name on the given OCI Distribution registry domain.
	//
	// If successful, the returned client should be preconfigured with
	// any credentials that are needed to access content from the
	// given repository. Implementers can assume that the client will
	// be used for only a short period after the call to this function,
	// so e.g. it's valid to use time-limited credentials with a validity
	// period on the order of 5 to 10 minutes if appropriate.
	//
	// Errors from this function should represent "operational-related"
	// situations like a failure to execute a credentials helper or
	// failure to issue a temporary access token, and will be presented
	// in the UI as a diagnostic with terminology like "Failed to access
	// OCI registry".
	getOCIRepositoryStore func(ctx context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error)

	// We keep an internal cache of the most-recently-instantiated
	// repository store object because in the common case there will
	// be call to AvailableVersions immediately followed by
	// PackageMeta with the same provider address and this avoids
	// us needing to repeat the translation and instantiation again.
	storeCacheMutex          sync.Mutex
	storeCacheProvider       addrs.Provider
	storeCacheStore          OCIRepositoryStore
	storeCacheRegistryDomain string
	storeCacheRepositoryName string
}

var _ Source = (*OCIRegistryMirrorSource)(nil)

// AvailableVersions implements Source.
func (o *OCIRegistryMirrorSource) AvailableVersions(ctx context.Context, provider addrs.Provider) (VersionList, Warnings, error) {
	store, _, _, err := o.getRepositoryStore(ctx, provider)
	if err != nil {
		return nil, nil, err
	}

	var ret VersionList
	err = store.Tags(ctx, "", func(tagNames []string) error {
		for _, tagName := range tagNames {
			// We're only interested in the subset of tag names that we can parse
			// as version numbers. However, the OCI tag name syntax does not allow
			// the "+" symbol that can appear in the version number syntax, so we
			// expect the underscore to stand in for that.
			maybeVersionStr := strings.ReplaceAll(tagName, "_", "+")
			version, err := versions.ParseVersion(maybeVersionStr)
			if err != nil {
				// Not a version tag, so ignored.
				continue
			}
			ret = append(ret, version)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, orasErrors.ErrNotFound) {
			// We treat "not found" as special because this function might be
			// called from [MultiSource.AvailableVersions] and that relies
			// on us returning this specific error type to represent that this
			// source can't handle the requested provider at all, so that it
			// can blend the results of multiple sources and only return an
			// error if _none_ of the eligible sources can handle the
			// requested provider. We assume that if the given provider address
			// translated to an OCI repository that doesn't exist then that
			// means this source does not know anything about that provider,
			// but other [MultiSource] candidates should still be offered it.
			return nil, nil, ErrProviderNotFound{
				Provider: provider,
			}
		}
		return nil, nil, fmt.Errorf("listing tags from OCI repository: %w", err)
	}
	ret.Sort()
	return ret, nil, nil
}

// PackageMeta implements Source.
func (o *OCIRegistryMirrorSource) PackageMeta(ctx context.Context, provider addrs.Provider, version Version, target Platform) (PackageMeta, error) {
	// Unfortunately we need to repeat our translation from provider address to
	// OCI repository address here, but getRepositoryStore has a cache that
	// allows us to reuse a previously-instantiated store if there are two
	// consecutive calls for the same provider, as is commonly true when first
	// calling AvailableVersions and then calling PackageMeta based on its result.
	store, registryDomain, repositoryName, err := o.getRepositoryStore(ctx, provider)
	if err != nil {
		return PackageMeta{}, err
	}

	// The overall process here is:
	// 1. Transform the version number into a tag name and resolve the descriptor
	//    associated with that tag name.
	// 2. Fetch the blob associated with that descriptor, which should be an
	//    index manifest giving a descriptor for each platform supported by this
	//    version of the provider.
	// 3. Choose the descriptor that matches the requested platform.
	// 4. Fetch the blob associated with that second descriptor, which should be
	//    an image manifest that includes a layer descriptor for the blob containing
	//    the actual zip package we need to fetch.
	// 5. Return a PackageMeta whose location is that zip blob, using [PackageOCIBlobArchive].

	// The errors from the following helper functions must not be wrapped by this
	// function because some of them are of specific types that get special
	// treatment by callers.
	indexDesc, err := fetchOCIDescriptorForVersion(ctx, version, store) // step 1
	if err != nil {
		return PackageMeta{}, err
	}
	index, err := fetchOCIIndexManifest(ctx, indexDesc, store) // step 2
	if err != nil {
		return PackageMeta{}, err
	}
	imageDesc, err := selectOCIImageManifest(index.Manifests, provider, version, target) // step 3
	if err != nil {
		return PackageMeta{}, err
	}
	imageManifest, err := fetchOCIImageManifest(ctx, imageDesc, store) // step 4
	if err != nil {
		return PackageMeta{}, err
	}
	blobDesc, err := selectOCILayerBlob(imageManifest.Layers) // a little more of step 4
	if err != nil {
		return PackageMeta{}, err
	}

	// The remainder of this is "step 5" from the overview above, adapting the information
	// we fetched to suit OpenTofu's provider installer API.

	// We'll announce the OpenTofu-style package hash that we're expecting as part of
	// the metadata. This isn't strictly necessary since OCI blobs are content-addressed
	// anyway and so we'll authenticate it using the same digest that identifies it
	// during the subsequent fetch, but this makes this source consistent with
	// [HTTPMirrorSource] and allows generating an explicit "checksum verified"
	// authentication result after install.
	expectedHash, err := hashFromOCIDigest(blobDesc.Digest)
	if err != nil {
		return PackageMeta{}, err
	}
	authentication := NewPackageHashAuthentication(target, []Hash{expectedHash})

	// If we got through all of the above then we seem to have found a suitable
	// package to install, but our job is only to describe its metadata.
	return PackageMeta{
		Provider:       provider,
		Version:        version,
		TargetPlatform: target,
		Location: PackageOCIBlobArchive{
			repoStore:      store,
			blobDescriptor: blobDesc,
			registryDomain: registryDomain,
			repositoryName: repositoryName,
		},
		Authentication: authentication,

		// "Filename" isn't really a meaningful concept in an OCI registry, but
		// that's okay because we don't care very much about it in OpenTofu
		// either and so we can just populate something plausible here. This
		// matches the way the package would be named in a traditional
		// OpenTofu provider network mirror.
		Filename: fmt.Sprintf("terraform-provider-%s_%s_%s_%s.zip", provider.Type, version, target.OS, target.Arch),

		// TODO: Define an optional annotation that can announce which protocol
		// versions are supported, so we can populate the ProtocolVersions
		// field and can fail early if the provider clearly doesn't support
		// any of the protocol versions that this OpenTofu version supports.
		// Omitting this field is okay though, since some other mirror sources
		// can't support it either: that just means that OpenTofu will discover
		// the compatibility problem only after executing the plugin, rather
		// than when installing it.
	}, nil
}

// ForDisplay implements Source.
func (o *OCIRegistryMirrorSource) ForDisplay(provider addrs.Provider) string {
	// We don't really have a good concise way to differentiate between
	// instances of this source because the mapping from provider source
	// address to OCI repository address is arbitrarily-defined by the
	// user, so we'll just settle for this right now since this is result
	// only typically used in error messages anyway. If this turns out to
	// be too vague in practice than hopefully whatever complaint caused
	// us to realize that will give a clue as to what additional information
	// is worth including here and where we might derive that information
	// from.
	return "OCI registry provider mirror"
}

func (o *OCIRegistryMirrorSource) getRepositoryStore(ctx context.Context, provider addrs.Provider) (store OCIRepositoryStore, registryDomain string, repositoryName string, err error) {
	o.storeCacheMutex.Lock()
	defer o.storeCacheMutex.Unlock()

	// If our cache is for the requested provider then we can reuse the store
	// we previously instantiated for this provider.
	if o.storeCacheProvider == provider {
		return o.storeCacheStore, o.storeCacheRegistryDomain, o.storeCacheRepositoryName, nil
	}

	// Otherwise we'll instantiate a new one and overwrite our cache with it.
	registryDomain, repositoryName, err = o.resolveOCIRepositoryAddr(ctx, provider)
	if err != nil {
		if notFoundErr, ok := err.(ErrProviderNotFound); ok {
			// [MultiSource] relies on this particular error type being returned
			// directly, without any wrapping.
			return nil, "", "", notFoundErr
		}
		return nil, "", "", fmt.Errorf("selecting OCI repository address: %w", err)
	}
	store, err = o.getOCIRepositoryStore(ctx, registryDomain, repositoryName)
	if err != nil {
		if errors.Is(err, orasErrors.ErrNotFound) {
			return nil, "", "", ErrProviderNotFound{
				Provider: provider,
			}
		}
		return nil, "", "", fmt.Errorf("accessing OCI registry at %s: %w", registryDomain, err)
	}
	o.storeCacheProvider = provider
	o.storeCacheStore = store
	o.storeCacheRegistryDomain = registryDomain
	o.storeCacheRepositoryName = repositoryName
	return store, registryDomain, repositoryName, nil
}

// OCIRepositoryStore is the interface used by [OCIRegistryMirrorSource] to
// interact with the content in a specific OCI repository.
type OCIRepositoryStore interface {
	// Tags lists the tag names available in the repository.
	//
	// The OCI Distribution protocol uses pagination for the tag list and so
	// the given function will be called for each page until either it returns
	// an error or there are no pages left to retrieve.
	//
	// "last" is a token used to begin at some point other than the start of
	// the list, but callers in this package always set it to an empty string
	// to represent intent to retrieve the entire list and so implementers are
	// allowed to return an error if "last" is non-empty.
	Tags(ctx context.Context, last string, fn func(tags []string) error) error

	// Resolve finds the descriptor associated with the given tag name in the
	// repository, if any.
	//
	// tagName MUST conform to the pattern defined for "<reference> as a tag"
	// from the OCI distribution specification, or the result is unspecified.
	Resolve(ctx context.Context, tagName string) (ociv1.Descriptor, error)

	// Fetch retrieves the content of a specific blob from the repository, identified
	// by the digest in the given descriptor.
	//
	// Implementations of this function tend not to check that the returned content
	// actually matches the digest and size in the descriptor, so callers MUST
	// verify that somehow themselves before making use of the resulting content.
	//
	// Callers MUST close the returned reader after using it, since it's typically
	// connected to an active network socket or file handle.
	Fetch(ctx context.Context, target ociv1.Descriptor) (io.ReadCloser, error)

	// The design of the above intentionally matches a subset of the interfaces
	// defined in the ORAS-Go library, but we have our own specific interface here
	// both to clearly define the minimal interface we depend on and so that our
	// use of ORAS-Go can be treated as an implementation detail rather than as
	// an API contract should we need to switch to a different approach in future.
	//
	// If you need to expand this while we're still relying on ORAS-Go, aim to
	// match the corresponding interface in that library if at all possible so
	// that we can minimize the amount of adapter code we need to write.
}

func fetchOCIDescriptorForVersion(ctx context.Context, version versions.Version, store OCIRepositoryStore) (ociv1.Descriptor, error) {
	// OCI tags don't support the "+" character used to mark the beginning of
	// build metadata in semver-style version numbers, so we expect a tag name
	// where those are replaced with "_".
	tagName := strings.ReplaceAll(version.String(), "+", "_")
	desc, err := store.Resolve(ctx, tagName)
	if err != nil {
		return ociv1.Descriptor{}, fmt.Errorf("resolving tag %q: %w", tagName, err)
	}
	if desc.ArtifactType != ociIndexManifestArtifactType {
		switch desc.ArtifactType {
		case "application/vnd.opentofu.provider-target":
			// We'll get here for an incorrectly-constructed artifact layout where
			// the tag refers directly to a specific platform's image manifest,
			// rather than to the multi-platform index manifest.
			return desc, fmt.Errorf("tag refers directly to image manifest, but OpenTofu providers require an index manifest for multi-platform support")
		case "application/vnd.opentofu.modulepkg":
			// If this happens to be using our artifact type for module packages then
			// we'll return a specialized error, since confusion between providers
			// and modules is common for those new to OpenTofu terminology.
			return desc, fmt.Errorf("selected OCI artifact is an OpenTofu module package, not a provider package")
		case "":
			// Prior to there being an explicit way to represent artifact types earlier
			// attempts to adapt OCI Distribution to non-container-image stuff used
			// custom layer media types instead. This case also deals with container images
			// themselves, which are essentially the "default" kind of artifact. We
			// haven't yet fetched the full manifest so we can't actually distinguish
			// these from the descriptor alone, and so this error message is generic.
			return desc, fmt.Errorf("unsupported OCI artifact type; is this a container image, rather than an OpenTofu provider?")
		default:
			// For any other artifact type we'll just mention it in the error message
			// and hope the reader can figure out what that artifact type represents.
			return desc, fmt.Errorf("unsupported OCI artifact type %q", desc.ArtifactType)
		}
	}
	if desc.MediaType != ociv1.MediaTypeImageIndex {
		switch desc.MediaType {
		case ociv1.MediaTypeImageManifest:
			return desc, fmt.Errorf("selected an OCI image manifest directly, but providers must be selected through a multi-platform index manifest")
		case ociv1.MediaTypeDescriptor:
			return desc, fmt.Errorf("found OCI descriptor but expected multi-platform index manifest")
		default:
			return desc, fmt.Errorf("unsupported media type %q for OCI index manifest", desc.MediaType)
		}
	}
	return desc, nil
}

func fetchOCIIndexManifest(ctx context.Context, desc ociv1.Descriptor, store OCIRepositoryStore) (*ociv1.Index, error) {
	manifestSrc, err := fetchOCIManifestBlob(ctx, desc, store)
	if err != nil {
		return nil, err
	}

	var manifest ociv1.Index
	err = json.Unmarshal(manifestSrc, &manifest)
	if err != nil {
		// As an aid to debugging, we'll check whether we seem to have retrieved
		// an image manifest instead of an index manifest, since an unmarshal
		// failure could prevent us from reaching the MediaType check below.
		var manifest ociv1.Manifest
		if err := json.Unmarshal(manifestSrc, &manifest); err == nil && manifest.MediaType == ociv1.MediaTypeImageManifest {
			return nil, fmt.Errorf("found image manifest but need index manifest")
		}
		return nil, fmt.Errorf("invalid manifest content: %w", err)
	}

	// Now we'll make sure that what we decoded seems vaguely sensible before we
	// return it. Callers are allowed to rely on these checks by verifying
	// that their provided descriptor specifies the wanted media and artifact
	// types before they call this function and then assuming that the result
	// definitely matches what they asked for.
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

func fetchOCIImageManifest(ctx context.Context, desc ociv1.Descriptor, store OCIRepositoryStore) (*ociv1.Manifest, error) {
	manifestSrc, err := fetchOCIManifestBlob(ctx, desc, store)
	if err != nil {
		return nil, err
	}

	var manifest ociv1.Manifest
	err = json.Unmarshal(manifestSrc, &manifest)
	if err != nil {
		// As an aid to debugging, we'll check whether we seem to have retrieved
		// an index manifest instead of an image manifest, since an unmarshal
		// failure could prevent us from reaching the MediaType check below.
		var manifest ociv1.Index
		if err := json.Unmarshal(manifestSrc, &manifest); err == nil && manifest.MediaType == ociv1.MediaTypeImageIndex {
			return nil, fmt.Errorf("found index manifest but need image manifest")
		}
		return nil, fmt.Errorf("invalid manifest content: %w", err)
	}

	// Now we'll make sure that what we decoded seems vaguely sensible before we
	// return it. Callers are allowed to rely on these checks by verifying
	// that their provided descriptor specifies the wanted media and artifact
	// types before they call this function and then assuming that the result
	// definitely matches what they asked for.
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

func selectOCIImageManifest(descs []ociv1.Descriptor, provider addrs.Provider, version versions.Version, target Platform) (ociv1.Descriptor, error) {
	foundManifests := 0
	foundWrongArtifactType := ""
	foundWrongPlatform := 0
	var selected ociv1.Descriptor
	for _, desc := range descs {
		if desc.ArtifactType != ociPackageManifestArtifactType {
			if desc.ArtifactType != "" {
				foundWrongArtifactType = desc.ArtifactType
			}
			continue // silently ignore anything that isn't claiming to be a provider package manifest
		}
		if desc.MediaType != ociv1.MediaTypeImageManifest {
			// If this descriptor claims to be for a provider target manifest then
			// it MUST be declared as being an image manifest.
			return selected, fmt.Errorf("provider image manifest has unsupported media type %q", desc.MediaType)
		}
		if desc.Platform == nil {
			return selected, fmt.Errorf("provider image manifest lacks the required platform constraints")
		}
		if desc.Platform.OSVersion != "" || desc.Platform.OS != target.OS || desc.Platform.Architecture != target.Arch {
			// We ignore manifests that aren't for the platform we're trying to match.
			// We also ignore manifests that specify a specific OS version because we
			// don't currently have any means to handle that, but we want to give
			// future OpenTofu versions the option of treating that as a more specific
			// match while leaving an OSVersion-free entry as a compatibility fallback.
			foundWrongPlatform++
			continue
		}
		// We have found a plausible candidate!
		foundManifests++
		selected = desc
	}
	if foundManifests == 0 {
		switch {
		case foundWrongPlatform > 0:
			// If all of the manifests were valid but none were eligible for this
			// platform then we assume that this is a valid provider that just
			// lacks support for the current platform, for which we have a special
			// error type.
			return selected, ErrPlatformNotSupported{
				Provider: provider,
				Version:  version,
				Platform: target,
			}
		case foundWrongArtifactType != "":
			// If we didn't find any manifests with the correct artifact type
			// targeting _any_ platform but we found at least one with a
			// different artifact type then we'll mention that here. We
			// arbitrarily just take whichever incorrect artifact type we
			// found most recently, since it's unlikely to get here so not
			// worth the complexity of trying to collect multiple.
			return selected, fmt.Errorf("provider image manifest has unsupported artifact type %q", foundWrongArtifactType)
		default:
			// This is a particularly annoying case: none of the declared
			// manifests have any artifact type _at all_. The most likely
			// cause of this is that the user has accidentally selected the
			// index manifest for a multi-platform container image, and so
			// we'll bias toward that explanation in the error message but
			// present it as a question to communicate that we're not sure.
			return selected, fmt.Errorf("provider image manifest has unsupported artifact type; is this a container image, rather than an OpenTofu provider?")
		}
	}
	if foundManifests > 1 {
		// There must be exactly one eligible manifest, to avoid ambiguity.
		return selected, fmt.Errorf("ambiguous manifest has multiple descriptors for platform %s", target)
	}
	return selected, nil
}

func selectOCILayerBlob(descs []ociv1.Descriptor) (ociv1.Descriptor, error) {
	foundBlobs := 0
	foundWrongMediaTypeBlobs := 0
	var selected ociv1.Descriptor
	for _, desc := range descs {
		if desc.ArtifactType != ociPackageArtifactType {
			continue
		}
		if desc.MediaType != ociPackageMediaType {
			// We silently ignore any "layer" that doesn't have both our expected
			// artifact type and media type so that future versions of OpenTofu
			// can potentially support additional archive formats, and so that
			// artifact authors can include other non-OpenTofu-related layers
			// in their manifests if needed... but we do still count them so that
			// we can hint about it in an error message below.
			foundWrongMediaTypeBlobs++
			continue
		}
		foundBlobs++
		selected = desc
	}
	if foundBlobs == 0 {
		if foundWrongMediaTypeBlobs > 0 {
			return selected, fmt.Errorf("image manifest contains no %q layers of type %q, but has other unsupported formats; this OCI artifact might be intended for a different version of OpenTofu", ociPackageArtifactType, ociPackageMediaType)
		}
		return selected, fmt.Errorf("image manifest contains no %q layers of type %q", ociPackageArtifactType, ociPackageMediaType)
	}
	if foundBlobs > 1 {
		// There must be exactly one eligible blob, to avoid ambiguity.
		return selected, fmt.Errorf("ambiguous manifest declares multiple eligible provider packages")
	}
	return selected, nil
}

func fetchOCIManifestBlob(ctx context.Context, desc ociv1.Descriptor, store OCIRepositoryStore) ([]byte, error) {
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

	return manifestSrc, nil
}
