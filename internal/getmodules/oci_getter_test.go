// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getmodules

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-getter"
	ociDigest "github.com/opencontainers/go-digest"
	ociSpecs "github.com/opencontainers/image-spec/specs-go"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	orasContent "oras.land/oras-go/v2/content"
	orasMemoryStore "oras.land/oras-go/v2/content/memory"
)

func TestGetterDecompressorsConsistent(t *testing.T) {
	// This test makes sure that the following three variables are
	// all defined consistently enough with one another to satisfy
	// the assumptions that ociDistributionGetter makes about them:
	// - goGetterDecompressors
	// - goGetterDecompressorMediaTypes
	// - ociBlobMediaTypePreference

	// Assumption 1: all entries in goGetterDecompressorMediaTypes have
	// a corresponding entry in goGetterDecompressors.
	for k, v := range goGetterDecompressorMediaTypes {
		_, ok := goGetterDecompressors[v]
		if !ok {
			t.Errorf("goGetterDecompressorMediaTypes[%q] refers to %q, which is not defined in goGetterDecompressors", k, v)
		}
	}

	// Assumption 2: every entry in goGetterDecompressorMediaTypes is
	// included somewhere in ociBlobMediaTypePreference, so that we
	// know which media type to prefer when multiple are present.
	if lenMT, lenPref := len(goGetterDecompressorMediaTypes), len(ociBlobMediaTypePreference); lenMT != lenPref {
		t.Errorf("goGetterDecompressorMediaTypes has %d elements, but ociBlobMediaTypePreference has %d; should be equal length", lenMT, lenPref)
	}
	for _, v := range ociBlobMediaTypePreference {
		_, ok := goGetterDecompressorMediaTypes[v]
		if !ok {
			t.Errorf("ociBlobMediaTypePreference includes %q, which is not present in goGetterDecompressorMediaTypes", v)
		}
	}
}

func TestOCIDistributionGetter(t *testing.T) {
	// For this test we'll use an in-memory-only repository store implementation,
	// although we need to use a local wrapper to work around a leaky abstraction
	// where the in-memory store doesn't behave quite the same as a real OCI
	// Distribution registry client would. :(
	//
	// In real use ociDistributionGetter is more likely to be used with ORAS-Go's
	// remote registry client implementation, but that's the caller's responsibility
	// to decide if so.
	mainStore := digestResolvingInMemoryOCIStore{
		orasMemoryStore.New(),
	}

	// We'll build some fake-but-valid module packages to put in this store so
	// that we can test various valid source address inputs.
	latestBlobDesc := ociPushFakeModulePackageBlob(t, "content of latest", mainStore)
	latestManifestDesc := ociPushFakeImageManifest(t, latestBlobDesc, ociIndexManifestArtifactType, mainStore)
	ociCreateTag(t, "latest", latestManifestDesc, mainStore)
	fooBlobDesc := ociPushFakeModulePackageBlob(t, "content of foo", mainStore)
	fooManifestDesc := ociPushFakeImageManifest(t, fooBlobDesc, ociIndexManifestArtifactType, mainStore)
	ociCreateTag(t, "foo", fooManifestDesc, mainStore)
	digestBlobDesc := ociPushFakeModulePackageBlob(t, "content of digest-only reference", mainStore)
	digestManifestDesc := ociPushFakeImageManifest(t, digestBlobDesc, ociIndexManifestArtifactType, mainStore)
	digestManifestDigestStr := digestManifestDesc.Digest.String()

	// We'll log the digests of the three manifests we're going to use in
	// the tests below just in case they appear as part of error messages,
	// so we can understand what failed.
	t.Logf("'latest' tag\nmanifest: %s\nblob:     %s", latestManifestDesc.Digest, latestBlobDesc.Digest)
	t.Logf("'foo' tag\nmanifest: %s\nblob:     %s", fooManifestDesc.Digest, fooBlobDesc.Digest)
	t.Logf("untagged manifest\nmanifest: %s\nblob:     %s", digestManifestDigestStr, digestBlobDesc.Digest)

	ociGetter := &ociDistributionGetter{
		getOCIRepositoryStore: func(ctx context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error) {
			if registryDomain != "example.com" {
				return nil, fmt.Errorf("no such registry")
			}
			switch repositoryName {
			case "main":
				return mainStore, nil
			case "empty":
				// We'll just return a completely empty store for this one
				return orasMemoryStore.New(), nil
			default:
				return nil, fmt.Errorf("no such repository")
			}
		},
	}

	tests := []struct {
		source          string
		wantFileContent string
		wantError       string
	}{
		{
			source:          "oci://example.com/main",
			wantFileContent: `content of latest`,
		},
		{
			source:          "oci://example.com/main?tag=foo",
			wantFileContent: `content of foo`,
		},
		{
			// NOTE: This particular test is currently relying on the workaround
			// applied by our store wrapper type [digestResolvingInMemoryOCIStore],
			// because the upstream ORAS-Go in-memory store does not implement
			// "Resolve" realistically as compared to a registry client implementation.
			source:          "oci://example.com/main?digest=" + digestManifestDigestStr,
			wantFileContent: `content of digest-only reference`,
		},

		// Various failure cases
		{
			source:    "oci://nonexist.example.com/boop",
			wantError: `error downloading 'oci://nonexist.example.com/boop': configuring client for nonexist.example.com/boop: no such registry`,
		},
		{
			source:    "oci://example.com/in$valid", // invalid repository name syntax, per OCI Distribution spec
			wantError: `error downloading 'oci://example.com/in$valid': invalid reference: invalid repository "in$valid"`,
		},
		{
			source:    "oci://example.com/empty",
			wantError: `error downloading 'oci://example.com/empty': resolving tag "latest": not found`,
		},
		{
			source:    "oci://example.com/empty?tag=baz",
			wantError: `error downloading 'oci://example.com/empty?tag=baz': resolving tag "baz": not found`,
		},
		{
			source:    "oci://example.com/empty?tag=in$valid", // invalid tag name syntax, per OCI distribution spec
			wantError: `error downloading 'oci://example.com/empty?tag=in%24valid': invalid reference: invalid tag "in$valid"`,
		},
		{
			source:    "oci://example.com/empty?digest=sha256:1d57d25084effd3fdfd902eca00020b34b1fb020253b84d7dd471301606015ac",
			wantError: `error downloading 'oci://example.com/empty?digest=sha256%3A1d57d25084effd3fdfd902eca00020b34b1fb020253b84d7dd471301606015ac': resolving digest "sha256:1d57d25084effd3fdfd902eca00020b34b1fb020253b84d7dd471301606015ac": not found`,
		},
		{
			source:    "oci://example.com/empty?tag=",
			wantError: `error downloading 'oci://example.com/empty?tag=': tag argument must not be empty`,
		},
		{
			source:    "oci://example.com/empty?tag=foo&tag=bar",
			wantError: `error downloading 'oci://example.com/empty?tag=foo&tag=bar': too many "tag" arguments`,
		},
		{
			source:    "oci://example.com/empty?digest=nope",
			wantError: `error downloading 'oci://example.com/empty?digest=nope': invalid digest: invalid checksum digest format`,
		},
		{
			source:    "oci://example.com/empty?digest=nope:nope",
			wantError: `error downloading 'oci://example.com/empty?digest=nope%3Anope': invalid digest: unsupported digest algorithm`,
		},
		{
			source:    "oci://example.com/empty?digest=",
			wantError: `error downloading 'oci://example.com/empty?digest=': digest argument must not be empty`,
		},
		{
			source:    "oci://example.com/empty?digest=nope&digest=nope",
			wantError: `error downloading 'oci://example.com/empty?digest=nope&digest=nope': too many "digest" arguments`,
		},
		{
			source:    "oci://example.com/empty?tag=foo&digest=sha256:1d57d25084effd3fdfd902eca00020b34b1fb020253b84d7dd471301606015ac",
			wantError: `error downloading 'oci://example.com/empty?digest=sha256%3A1d57d25084effd3fdfd902eca00020b34b1fb020253b84d7dd471301606015ac&tag=foo': cannot set both "tag" and "digest" arguments`,
		},
		{
			source:    "oci://example.com/empty?tag=foo&other=bar",
			wantError: `error downloading 'oci://example.com/empty?other=bar&tag=foo': unsupported argument "other"`,
		},
		{
			source:    "oci://example.com/empty?archive=zip",
			wantError: `the "archive" argument is not allowed for OCI sources, because the archive format is detected automatically from the image manifest`,
		},
	}

	for _, test := range tests {
		t.Run(test.source, func(t *testing.T) {
			instPath := t.TempDir()
			client := getter.Client{
				Src: test.source,
				Dst: instPath,
				Pwd: instPath,

				Mode: getter.ClientModeDir,

				Detectors: goGetterNoDetectors,
				Getters: map[string]getter.Getter{
					"oci": ociGetter,
				},
				Ctx: t.Context(),
			}
			err := client.Get()

			if test.wantError != "" {
				if err == nil {
					t.Fatalf("unexpected success\nwant error: %s", test.wantError)
				}
				if got := err.Error(); got != test.wantError {
					t.Fatalf("unexpected error\ngot:  %s\nwant: %s", got, test.wantError)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if test.wantFileContent == "" {
				return // no file content test required
			}

			gotContentRaw, err := os.ReadFile(filepath.Join(instPath, "test_content.txt"))
			if err != nil {
				t.Fatal(err)
			}
			gotContent := string(bytes.TrimSpace(gotContentRaw))
			if gotContent != test.wantFileContent {
				t.Errorf("wrong file content after successful install\ngot:  %s\nwant: %s", gotContent, test.wantFileContent)
			}
		})
	}

}

func ociPushFakeModulePackageBlob(t *testing.T, fakeContent string, store orasContent.Pusher) ociv1.Descriptor {
	t.Helper()

	var buf bytes.Buffer
	zr := zip.NewWriter(&buf)
	fw, err := zr.Create("test_content.txt")
	if err != nil {
		t.Fatalf("can't create file in fake module package: %s", err)
	}
	n, err := io.WriteString(fw, fakeContent)
	if err != nil {
		t.Fatalf("can't write to file in fake module package: %s", err)
	}
	if n != len(fakeContent) {
		t.Fatalf("incomplete write of fake content")
	}
	zr.Close()

	desc := ociv1.Descriptor{
		MediaType: "archive/zip",
		Digest:    ociDigest.FromBytes(buf.Bytes()),
		Size:      int64(buf.Len()),
	}
	err = store.Push(t.Context(), desc, &buf)
	if err != nil {
		t.Fatalf("can't push blob to store: %s", err)
	}
	return desc
}

func ociPushFakeImageManifest(t *testing.T, layerDesc ociv1.Descriptor, artifactType string, store orasContent.Pusher) ociv1.Descriptor {
	t.Helper()

	manifest := &ociv1.Manifest{
		Versioned: ociSpecs.Versioned{
			SchemaVersion: 2,
		},
		MediaType:    ociv1.MediaTypeImageManifest,
		ArtifactType: artifactType,
		Config:       ociv1.DescriptorEmptyJSON,
		Layers:       []ociv1.Descriptor{layerDesc},
	}
	manifestSrc, err := json.Marshal(manifest)
	if err != nil {
		t.Errorf("can't serialize manifest: %s", err)
	}

	desc := ociv1.Descriptor{
		MediaType:    manifest.MediaType,
		ArtifactType: manifest.ArtifactType,
		Digest:       ociDigest.FromBytes(manifestSrc),
		Size:         int64(len(manifestSrc)),
	}
	err = store.Push(t.Context(), desc, bytes.NewReader(manifestSrc))
	if err != nil {
		t.Fatalf("can't push manifest to store: %s", err)
	}
	return desc
}

func ociCreateTag(t *testing.T, tagName string, desc ociv1.Descriptor, store orasContent.Tagger) {
	t.Helper()

	err := store.Tag(t.Context(), desc, tagName)
	if err != nil {
		t.Fatalf("can't create tag %q: %s", tagName, err)
	}
}

// digestResolvingInMemoryOCIStore is a moderately-ugly hack to make
// ORAS-Go's in memory store behave slightly more like a realistic
// OCI Distribution registry server by allowing the resolution of
// raw digests into descriptors, whereas the upstream in-memory
// implementation only allows resolving tags.
//
// It's unfortunate that the various ORAS-Go "fake" implementations
// are not realistic as compared to a real registry, but this minor
// concession allows us to avoid all of the pain of running a _real_
// registry server to support our unit tests.
//
// (We also use an end-to-end test in the command/e2etest package
// that exercises similar behavior with ORAS-Go's real registry
// client implementation, to give us some insurance that this
// workaround stays realistic enough.)
type digestResolvingInMemoryOCIStore struct {
	*orasMemoryStore.Store
}

var _ OCIRepositoryStore = digestResolvingInMemoryOCIStore{}

func (s digestResolvingInMemoryOCIStore) Push(ctx context.Context, expected ociv1.Descriptor, content io.Reader) error {
	// First we'll delegate to the upstream implementation to get the blob
	// actually saved in the store.
	err := s.Store.Push(ctx, expected, content)
	if err != nil {
		return err
	}

	// If storage was successful _and_ if the descriptor suggests that this
	// was intended to be a manifest blob then we'll create a fake "tag"
	// whose name matches the digest, which is just enough to trick
	// the "Resolve" method into handling the lookup the same way a real
	// OCI Registry client would handle it.
	if expected.MediaType == ociv1.MediaTypeImageManifest {
		err = s.Store.Tag(ctx, expected, expected.Digest.String())
		if err != nil {
			return fmt.Errorf("while creating a weird tag to fake looking up by digest: %w", err)
		}
	}

	return nil
}
