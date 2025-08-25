// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getmodules

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	getter "github.com/hashicorp/go-getter"
	ociDigest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opentofu/opentofu/internal/tracing"
	otelAttr "go.opentelemetry.io/otel/attribute"
	otelTrace "go.opentelemetry.io/otel/trace"
	orasContent "oras.land/oras-go/v2/content"
	orasRegistry "oras.land/oras-go/v2/registry"
)

// ociImageManifestArtifactType is the artifact type we expect for the image
// manifest describing an OpenTofu module package.
const ociIndexManifestArtifactType = "application/vnd.opentofu.modulepkg"

// ociImageManifestSizeLimit is the maximum size of artifact manifest (aka "image
// manifest") we'll accept. This 4MiB value matches the recommended limit for
// repositories to accept on push from the OCI Distribution v1.1 spec:
//
//	https://github.com/opencontainers/distribution-spec/blob/v1.1.0/spec.md#pushing-manifests
const ociImageManifestSizeLimitMiB = 4

// ociBlobMediaTypePreference describes our preference order for the media
// types of OCI blobs representing module packages.
//
// All elements of this slice must correspond to keys in
// [goGetterDecompressorMediaTypes], which in turn define which go-getter
// decompressor to use to extract an archive of each type. Furthermore,
// this must contain an element for every key in that map.
var ociBlobMediaTypePreference = []string{
	"archive/zip",
}

// ociDistributionGetter is an implementation of [getter.Getter] that
// obtains module packages from OCI distribution registries.
//
// Because this implementation lives inside OpenTofu rather than upstream
// go-getter, it intentionally focuses only on the subset of go-getter
// functionality that OpenTofu's module installer uses. If we do someday
// decide to submit this upstream it will need some further work to
// support additional capabilities that other go-getter callers rely on.
type ociDistributionGetter struct {
	getOCIRepositoryStore func(ctx context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error)

	// go-getter sets this by calling our SetClient method whenever
	// the client is configured, which happens automatically
	// when it Get method is called.
	client *getter.Client
}

var _ getter.Getter = (*ociDistributionGetter)(nil)

// Get implements getter.Getter.
func (g *ociDistributionGetter) Get(destDir string, url *url.URL) error {
	ctx := g.context()

	ctx, span := tracing.Tracer().Start(
		ctx, "Fetch 'oci' module package",
		otelTrace.WithAttributes(
			otelAttr.String("opentofu.module.source", url.String()),
			otelAttr.String("opentofu.module.local_dir", destDir),
		),
	)
	defer span.End()

	ref, err := g.resolveRepositoryRef(url)
	if err != nil {
		tracing.SetSpanError(span, err)
		return err
	}
	store, err := g.getOCIRepositoryStore(ctx, ref.Registry, ref.Repository)
	if err != nil {
		err := fmt.Errorf("configuring client for %s: %w", ref, err)
		tracing.SetSpanError(span, err)
		return err
	}
	manifestDesc, err := g.resolveManifestDescriptor(ctx, ref, url.Query(), store)
	if err != nil {
		tracing.SetSpanError(span, err)
		return err
	}
	manifest, err := fetchOCIImageManifest(ctx, manifestDesc, store)
	if err != nil {
		tracing.SetSpanError(span, err)
		return err
	}
	pkgDesc, err := selectOCILayerBlob(manifest.Layers)
	if err != nil {
		tracing.SetSpanError(span, err)
		return err
	}
	decompKey := goGetterDecompressorMediaTypes[pkgDesc.MediaType]
	decomp := goGetterDecompressors[decompKey]
	if decomp == nil {
		// Should not get here if selectOCILayerBlob is implemented correctly.
		err := fmt.Errorf("no decompressor available for media type %q", pkgDesc.MediaType)
		tracing.SetSpanError(span, err)
		return err
	}
	tempFile, err := fetchOCIBlobToTemporaryFile(ctx, pkgDesc, store)
	if err != nil {
		tracing.SetSpanError(span, err)
		return err
	}
	defer os.Remove(tempFile)

	var umask os.FileMode
	if g.client != nil {
		umask = g.client.Umask
	}
	err = decomp.Decompress(destDir, tempFile, true, umask)
	if err != nil {
		err := fmt.Errorf("decompressing package into %s: %w", destDir, err)
		tracing.SetSpanError(span, err)
		return err
	}
	return nil
}

// GetFile implements getter.Getter.
func (g *ociDistributionGetter) GetFile(string, *url.URL) error {
	// With how OpenTofu uses go-getter we can only get in here if
	// the source address string includes go-getter's special
	// reserved "archive" query string argument, which causes
	// go-getter itself to arrange for extracting the downloaded
	// archive.
	//
	// This getter includes its own archive handling that reacts
	// directly to the mediaType declared in the OCI image
	// manifest, so the "archive" query string argument does
	// not need to be supported here. (It's primarily intended
	// for general-purpose arbitrary file fetching getters like
	// the HTTP getter.)
	return fmt.Errorf("the \"archive\" argument is not allowed for OCI sources, because the archive format is detected automatically from the image manifest")
}

// ClientMode implements getter.Getter.
func (g *ociDistributionGetter) ClientMode(*url.URL) (getter.ClientMode, error) {
	// This getter only supports "dir" mode, meaning that
	// it populates a directory based on the content of the
	// retrieved archive rather than _just_ retrieving the
	// archive. In practice this isn't actually used in
	// OpenTofu, because OpenTofu _always_ asks for
	// go-getter to populate a directory, but we're required
	// to implement this method to satisfy the Getter interface.
	return getter.ClientModeDir, nil
}

// SetClient implements getter.Getter.
func (g *ociDistributionGetter) SetClient(client *getter.Client) {
	g.client = client
}

func (g *ociDistributionGetter) context() context.Context {
	// go-getter was designed long before [context.Context] existed, so
	// it passes us a context only indirectly through the client.
	if g == nil || g.client == nil {
		return context.Background() // For robustness, but client should always be set in practice
	}
	return g.client.Ctx
}

func (g *ociDistributionGetter) resolveRepositoryRef(url *url.URL) (*orasRegistry.Reference, error) {
	if !url.IsAbs() {
		// Should not get here, but just for robustness since go-getter
		// is a little quirky in what it allows users to express
		// with its source address syntax.
		return nil, fmt.Errorf("oci source type requires an absolute URL")
	}
	if url.Scheme != "oci" {
		// We can potentially get in here if the author writes a bizarre
		// source address with an explicit getter separate from the
		// scheme, like oci::https://example.com/ .
		return nil, fmt.Errorf("oci source type only supports oci URL scheme")
		// Most authors will hopefully write just oci:// as the scheme part
		// and let go-getter automatically select the oci getter based on
		// that, in which case the scheme will be set correctly.
	}

	// As usual, registryDomainName can optionally include a trailing
	// port number separated by a colon despite the name. This is a
	// standard naming quirk in OCI Distribution implementations,
	// since most addresses do not use an explicit port number.
	registryDomainName := url.Host

	// The OpenTofu module installer has already stripped off any
	// "subdir" part of the path before calling us, so we can assume
	// that the entire path is intended to be the repository name,
	// except that the leading slash should not be included.
	repositoryName := strings.TrimPrefix(url.Path, "/")

	// We'll borrow ORAS-Go's implementation to validate the address elements.
	ref := &orasRegistry.Reference{
		Registry:   registryDomainName,
		Repository: repositoryName,
	}
	if err := ref.Validate(); err != nil {
		// ORAS-Go's error messages already include an "invalid reference:"
		// prefix, so we won't add anything new here.
		return nil, err
	}
	return ref, nil
}

func (g *ociDistributionGetter) resolveManifestDescriptor(ctx context.Context, ref *orasRegistry.Reference, query url.Values, store OCIRepositoryStore) (desc ociv1.Descriptor, err error) {
	ctx, span := tracing.Tracer().Start(
		ctx, "Resolve reference",
		otelTrace.WithAttributes(
			otelAttr.String("opentofu.oci.registry.domain", ref.Registry),
			otelAttr.String("opentofu.oci.repository.name", ref.Repository),
		),
	)
	defer span.End()
	prepErr := func(err error) error {
		tracing.SetSpanError(span, err)
		return err
	}

	var unsupportedArgs []string
	var wantTag string
	var wantDigest ociDigest.Digest
	for name, values := range query {
		if len(values) > 1 {
			return ociv1.Descriptor{}, prepErr(fmt.Errorf("too many %q arguments", name))
		}
		value := values[0]
		switch name {
		case "tag":
			if value == "" {
				return ociv1.Descriptor{}, prepErr(fmt.Errorf("tag argument must not be empty"))
			}
			tagRef := *ref           // shallow copy so we can modify the reference field
			tagRef.Reference = value // We'll again borrow the ORAS-Go validation for this
			if err := tagRef.ValidateReferenceAsTag(); err != nil {
				return ociv1.Descriptor{}, prepErr(err) // message includes suitable context prefix already
			}
			wantTag = value
		case "digest":
			if value == "" {
				return ociv1.Descriptor{}, prepErr(fmt.Errorf("digest argument must not be empty"))
			}
			d, err := ociDigest.Parse(value)
			if err != nil {
				return ociv1.Descriptor{}, prepErr(fmt.Errorf("invalid digest: %s", err))
			}
			wantDigest = d
		default:
			unsupportedArgs = append(unsupportedArgs, name)
		}
	}
	if len(unsupportedArgs) == 1 {
		return ociv1.Descriptor{}, prepErr(fmt.Errorf("unsupported argument %q", unsupportedArgs[0]))
	} else if len(unsupportedArgs) >= 2 {
		return ociv1.Descriptor{}, prepErr(fmt.Errorf("unsupported arguments: %s", strings.Join(unsupportedArgs, ", ")))
	}
	if wantTag != "" && wantDigest != "" {
		return ociv1.Descriptor{}, prepErr(fmt.Errorf("cannot set both \"tag\" and \"digest\" arguments"))
	}
	if wantTag == "" && wantDigest == "" {
		wantTag = "latest" // default tag to use if no arguments are present
	}

	if wantTag != "" {
		// If we're starting with a tag name then we need to query the
		// repository to find out which digest is currently selected.
		span.SetAttributes(
			otelAttr.String("opentofu.oci.reference.tag", wantTag),
		)
		desc, err = store.Resolve(ctx, wantTag)
		if err != nil {
			return ociv1.Descriptor{}, prepErr(fmt.Errorf("resolving tag: %w", err))
		}
	} else {
		// If we're requesting a specific digest then we still need to
		// resolve to know the size and mediaType.
		// NOTE: The following is supported for the "real" OCI Distribution
		// protocol implementatino of ORAS-Go's "resolver" API, but
		// most of the other implementations only allow resolving by tag,
		// and so we can't exercise this specific case from unit tests
		// using in-memory or on-disk fakes. :(
		span.SetAttributes(
			otelAttr.String("opentofu.oci.reference.digest", wantDigest.String()),
		)
		desc, err = store.Resolve(ctx, wantDigest.String())
		if err != nil {
			return ociv1.Descriptor{}, prepErr(fmt.Errorf("resolving digest: %w", err))
		}
	}

	span.SetAttributes(
		otelAttr.String("oci.manifest.digest", desc.Digest.String()),
		otelAttr.String("opentofu.oci.manifest.media_type", desc.MediaType),
		otelAttr.Int64("opentofu.oci.manifest.size", desc.Size),
	)

	// The initial request is only required to return a "plain" descriptor,
	// with only MediaType+Digest+Size, so we can verify the media type
	// here but we'll need to wait until we fetch the manifest to verify
	// the ArtifactType and any other details.
	if desc.MediaType != ociv1.MediaTypeImageManifest {
		return ociv1.Descriptor{}, prepErr(fmt.Errorf("selected object is not an OCI image manifest"))
	}

	// We always expect ArtifactType to be set to our OpenTofu-specific type,
	// so we can reject attempts to install other kinds of artifact.
	desc.ArtifactType = ociIndexManifestArtifactType

	return desc, nil
}

func fetchOCIImageManifest(ctx context.Context, desc ociv1.Descriptor, store OCIRepositoryStore) (*ociv1.Manifest, error) {
	ctx, span := tracing.Tracer().Start(
		ctx, "Fetch manifest",
		otelTrace.WithAttributes(
			otelAttr.String("oci.manifest.digest", desc.Digest.String()),
			otelAttr.Int64("opentofu.oci.manifest.size", desc.Size),
		),
	)
	defer span.End()
	prepErr := func(err error) error {
		tracing.SetSpanError(span, err)
		return err
	}

	manifestSrc, err := fetchOCIManifestBlob(ctx, desc, store)
	if err != nil {
		return nil, prepErr(err)
	}

	var manifest ociv1.Manifest
	err = json.Unmarshal(manifestSrc, &manifest)
	if err != nil {
		// As an aid to debugging, we'll check whether we seem to have retrieved
		// an index manifest instead of an image manifest, since an unmarshal
		// failure could prevent us from reaching the MediaType check below.
		var manifest ociv1.Index
		if err := json.Unmarshal(manifestSrc, &manifest); err == nil && manifest.MediaType == ociv1.MediaTypeImageIndex {
			return nil, prepErr(fmt.Errorf("found index manifest but need image manifest"))
		}
		return nil, prepErr(fmt.Errorf("invalid manifest content: %w", err))
	}

	span.SetAttributes(
		otelAttr.String("opentofu.oci.manifest.media_type", desc.MediaType),
		otelAttr.String("opentofu.oci.manifest.artifact_type", desc.ArtifactType),
	)

	// Now we'll make sure that what we decoded seems vaguely sensible before we
	// return it. Callers are allowed to rely on these checks by verifying
	// that their provided descriptor specifies the wanted media and artifact
	// types before they call this function and then assuming that the result
	// definitely matches what they asked for.
	if manifest.MediaType != desc.MediaType {
		return nil, prepErr(fmt.Errorf("unexpected manifest media type %q", manifest.MediaType))
	}
	if manifest.ArtifactType != desc.ArtifactType {
		return nil, prepErr(fmt.Errorf("unexpected artifact type %q", manifest.ArtifactType))
	}
	// We intentionally leave everything else loose so that we'll have flexibility
	// to extend this format in backward-compatible ways in future OpenTofu versions.
	return &manifest, nil
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
	// and we also need to parse that data as JSON. We impose a reasonable upper
	// limit on manifest size, so we'll make our life easier for both by buffering
	// the whole manifest in RAM.
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

func selectOCILayerBlob(descs []ociv1.Descriptor) (ociv1.Descriptor, error) {
	foundBlobs := make(map[string]ociv1.Descriptor, len(goGetterDecompressorMediaTypes))
	foundWrongMediaTypeBlobs := 0
	for _, desc := range descs {
		if _, ok := goGetterDecompressorMediaTypes[desc.MediaType]; ok {
			if _, exists := foundBlobs[desc.MediaType]; exists {
				// We only allow one layer for each of our supported media types
				// because otherwise we'd have no way to choose between them.
				return ociv1.Descriptor{}, fmt.Errorf("multiple layers with media type %q", desc.MediaType)
			}
			foundBlobs[desc.MediaType] = desc
		} else {
			// We silently ignore any "layer" that doesn't use one of our
			// supported media types so that future versions of OpenTofu
			// can potentially support additional archive formats,
			// but we do still count them so that we can hint about
			// potential problems in an error message below.
			foundWrongMediaTypeBlobs++
		}
	}
	if len(foundBlobs) == 0 {
		if foundWrongMediaTypeBlobs > 0 {
			return ociv1.Descriptor{}, fmt.Errorf("image manifest contains no layers of types supported as module packages by OpenTofu, but has other unsupported formats; this OCI artifact might be intended for a different version of OpenTofu")
		}
		return ociv1.Descriptor{}, fmt.Errorf("image manifest contains no layers of types supported as module packages by OpenTofu")
	}
	for _, maybeType := range ociBlobMediaTypePreference {
		ret, ok := foundBlobs[maybeType]
		if ok {
			return ret, nil
		}
	}
	// We should not get here if goGetterDecompressorMediaTypes and
	// ociBlobMediaTypePreference have been maintained consistently,
	// but we'll return an error here anyway just to be robust.
	return ociv1.Descriptor{}, fmt.Errorf("image manifest contains no layers of types supported as module packages by OpenTofu")
}

// fetchOCIBlobToTemporaryFile uses the given ORAS fetcher to pull the content of the
// blob described by "desc" into a temporary file on the local filesystem, and
// then returns the path to that file.
//
// It is the caller's responsibility to delete the temporary file once it's no longer
// needed.
func fetchOCIBlobToTemporaryFile(ctx context.Context, desc ociv1.Descriptor, store orasContent.Fetcher) (tempFile string, err error) {
	ctx, span := tracing.Tracer().Start(
		ctx, "Fetch module package",
		otelTrace.WithAttributes(
			otelAttr.String("opentofu.oci.blob.digest", desc.Digest.String()),
			otelAttr.String("opentofu.oci.blob.media_type", desc.MediaType),
			otelAttr.Int64("opentofu.oci.blob.size", desc.Size),
		),
	)
	defer span.End()

	f, err := os.CreateTemp("", "opentofu-module")
	if err != nil {
		err := fmt.Errorf("failed to open temporary file: %w", err)
		tracing.SetSpanError(span, err)
		return "", err
	}
	tempFile = f.Name()
	defer func() {
		tracing.SetSpanError(span, err)
		// If we're returning an error then the caller won't make use of the
		// file we've created, so we'll make a best effort to proactively
		// remove it. If we return a nil error then it's the caller's
		// responsibility to remove the file once it's no longer needed.
		if err != nil {
			os.Remove(f.Name())
		}
	}()

	readCloser, err := store.Fetch(ctx, desc)
	if err != nil {
		return "", err
	}
	defer readCloser.Close()

	// We'll borrow go-getter's "cancelable copy" implementation here so that
	// the download can potentially be interrupted partway through.
	// orasContent.VerifyReader allows us to also check that the content
	// matches the digest and size given in the descriptor without having
	// to buffer the whole blob into RAM at once.
	v := orasContent.NewVerifyReader(readCloser, desc)
	_, err = getter.Copy(ctx, f, v)
	f.Close() // we're done using the filehandle now, even if the copy failed
	if err != nil {
		return "", err
	}
	if err := v.Verify(); err != nil {
		return "", fmt.Errorf("invalid blob returned from registry: %w", err)
	}

	return tempFile, nil
}

// OCIRepositoryStore is the interface that [PackageFetcher] uses to
// interact with a single OCI Distribution repository when installing
// a remote module using the "oci" scheme.
//
// Implementations of this interface are returned by
// [PackageFetcherEnvironment.OCIRepositoryStore].
type OCIRepositoryStore interface {
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
