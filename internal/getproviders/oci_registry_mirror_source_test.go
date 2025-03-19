// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	svchost "github.com/hashicorp/terraform-svchost"
	ociDigest "github.com/opencontainers/go-digest"
	ociSpecs "github.com/opencontainers/image-spec/specs-go"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	orasContent "oras.land/oras-go/v2/content"
	orasOCI "oras.land/oras-go/v2/content/oci"
	orasErrors "oras.land/oras-go/v2/errdef"

	"github.com/opentofu/opentofu/internal/addrs"
)

func TestOCIRegistryMirrorSource(t *testing.T) {
	// For unit test purposes we use a local-filesystem-based repository
	// rather than a remote registry repository. We use an on-disk
	// fake rather than an in-memory fake for this one only because
	// none of ORAS-Go's in-memory implementations implement the
	// TagLister interface.
	store, err := orasOCI.NewWithContext(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// We'll create a few separate sets of blob+manifests so that we'll
	// have multiple tags to choose from and can test that we selected
	// the right one.
	fakePlatforms := []Platform{
		{OS: "amigaos", Arch: "m86k"},
		{OS: "tos", Arch: "m86k"},
	}
	for patchVersion := range 3 {
		version := Version{Major: 1, Minor: 0, Patch: uint64(patchVersion)}
		if patchVersion == 2 {
			// We'll include one version with build metadata so that we can
			// test the transformation into a valid OCI Distribution tag.
			version.Metadata = "foo.1"
		}
		manifestDescs := make([]ociv1.Descriptor, 0, len(fakePlatforms))
		for _, platform := range fakePlatforms {
			// We use different placeholder content for the fake executable in each
			// version/platform combination so that once installation is complete
			// we can check that we actually installed the correct one.
			fakePackageBytes := makePlaceholderProviderPackageZip(t, fmt.Sprintf("placeholder executable for v%s on %s", version, platform))
			packageDesc := pushOCIBlob(t, "archive/zip", "application/vnd.opentofu.providerpkg", fakePackageBytes, store)
			manifestDesc := pushOCIImageManifest(t, &ociv1.Manifest{
				Versioned:    ociSpecs.Versioned{SchemaVersion: 2},
				MediaType:    ociv1.MediaTypeImageManifest,
				ArtifactType: "application/vnd.opentofu.provider-target",
				Config:       ociv1.DescriptorEmptyJSON,
				Layers: []ociv1.Descriptor{
					packageDesc,
				},
			}, store)
			manifestDesc.Platform = &ociv1.Platform{
				Architecture: platform.Arch,
				OS:           platform.OS,
			}
			manifestDescs = append(manifestDescs, manifestDesc)
		}
		indexDesc := pushOCIIndexManifest(t, &ociv1.Index{
			Versioned:    ociSpecs.Versioned{SchemaVersion: 2},
			MediaType:    ociv1.MediaTypeImageIndex,
			ArtifactType: "application/vnd.opentofu.provider",
			Manifests:    manifestDescs,
		}, store)
		tagName := strings.ReplaceAll(version.String(), "+", "_") // tag names aren't allowed to contain "+", so we use "_" instead
		createOCITag(t, tagName, indexDesc, store)
	}
	// One additional tag that intentionally doesn't conform to the version
	// number pattern so that we can make sure it gets silently ignored when
	// enumerating versions, rather than causing an error. The content of
	// this one is irrelevant because OpenTofu can't interact with it.
	pushOCIBlob(t, ociv1.DescriptorEmptyJSON.MediaType, ociv1.DescriptorEmptyJSON.ArtifactType, ociv1.DescriptorEmptyJSON.Data, store)
	createOCITag(t, "latest", ociv1.DescriptorEmptyJSON, store)
	// We also have a separate store that contains OCI content that
	// intentionally doesn't match our provider-specific manifest layout,
	// so that we can test our error handling for mistakes like accidentally
	// selecting something that was intended to be a container image.
	// This part is factored out into a separate function because it's
	// not the main case of this test and is much less systematic than the
	// above setup code.
	wrongStore, wrongStoreLookups := makeFakeOCIRepositoryWithNonProviderContent(t)

	// At this point our store should contain three tags: 1.0.0, 1.0.1, and 1.0.2.
	// Each tag refers to manifests representing a provider that supports the platforms amigaos_m68k and tos_m68k.
	// We'll set up our source to interact with the fake local repository we just set up.
	source := &OCIRegistryMirrorSource{
		resolveOCIRepositoryAddr: func(ctx context.Context, addr addrs.Provider) (registryDomain string, repositoryName string, err error) {
			if addr.Hostname != svchost.Hostname("example.com") {
				// We'll return [ErrProviderNotFound] here to satisfy the documented contract
				// that the source will return that error type in particular when asked for
				// a provider the mapping can't support, since [MultiSource] relies on that
				// to be able to successfully blend results from multiple sources that each
				// support a different subset of providers.
				return "", "", ErrProviderNotFound{
					Provider: addr,
				}
			}
			return "example.com", fmt.Sprintf("%s_%s", addr.Namespace, addr.Type), nil
		},
		getOCIRepositoryStore: func(ctx context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error) {
			if registryDomain != "example.com" {
				// This result mimicks how ORAS-Go represents a missing repository.
				return nil, orasErrors.ErrNotFound
			}
			switch repositoryName {
			case "foo_bar":
				// example.com/foo/bar is our repository containing valid provider
				// artifacts.
				return store, nil
			case "not_provider":
				// example.com/not/provider is our repository containing an assortment
				// of not-actually-OpenTofu-provider artifacts.
				return wrongStore, nil
			default:
				// All other addresses represent repositories that don't exist at all.
				return nil, orasErrors.ErrNotFound
			}
		},
	}

	// The following subtests all interact in some way with the faked source we've just
	// set up, although some of them use it more deeply than others.
	t.Run("happy path", func(t *testing.T) {
		fakeProvider := addrs.MustParseProviderSourceString("example.com/foo/bar")
		wantVersions := VersionList{
			MustParseVersion("1.0.0"),
			MustParseVersion("1.0.1"),
			MustParseVersion("1.0.2+foo.1"),
			// NOTE: The tag called "latest" is silently ignored because it's not
			// parsable as a version number.
		}
		gotVersions, warns, err := source.AvailableVersions(t.Context(), fakeProvider)
		if err != nil {
			t.Fatal(err)
		}
		if len(warns) != 0 {
			t.Errorf("unexpected warnings: %#v", warns)
		}
		if diff := cmp.Diff(wantVersions, gotVersions); diff != "" {
			t.Error("wrong versions\n" + diff)
		}

		testVersion := wantVersions[1]
		testPlatform := fakePlatforms[0]
		meta, err := source.PackageMeta(t.Context(), fakeProvider, testVersion, testPlatform)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := meta.Provider, fakeProvider; got != want {
			t.Errorf("wrong provider\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := meta.Version, testVersion; got != want {
			t.Errorf("wrong version\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := meta.TargetPlatform, testPlatform; got != want {
			t.Errorf("wrong platform\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := meta.Filename, "terraform-provider-bar_1.0.1_amigaos_m86k.zip"; got != want {
			// NOTE: This field doesn't actually really matter; OpenTofu doesn't
			// do anything significant with it so we're just populating it with
			// a plausible name to stay consistent with the other sources.
			t.Errorf("wrong filename\ngot:  %s\nwant: %s", got, want)
		}

		// We're not going to exhaustively test the Location field here because
		// this type has its own tests in [TestPackageOCIBlobArchive], but we
		// will make some use of it here just because otherwise we'd end up
		// reimplementing much of what [PackageOCIBlobArchive] does inline here.
		loc, ok := meta.Location.(PackageOCIBlobArchive)
		if !ok {
			t.Fatalf("wrong type for meta.Location\ngot:  %T, want %T", meta.Location, loc)
		}
		pkgDir := t.TempDir()
		authResult, err := loc.InstallProviderPackage(t.Context(), meta, pkgDir, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := authResult.result, verifiedChecksum; got != want {
			t.Errorf("wrong authentication result\ngot:  %#v\nwant: %#v", got, want)
		}
		exeContent, err := os.ReadFile(filepath.Join(pkgDir, "terraform-provider-foo"))
		if err != nil {
			t.Fatalf("failed to read fake provider executable file: %s", err)
		}
		if got, want := string(exeContent), "placeholder executable for v1.0.1 on amigaos_m86k"; got != want {
			t.Errorf("wrong content in fake executable file\ngot:  %q\nwant: %q", got, want)
		}
	})
	t.Run("version number with build metadata", func(t *testing.T) {
		// The "happy path" test verifies that we can list the tags and convert a tag name
		// containing "_" into a semver version number with "+" for build metadata. This
		// test case builds on that by making sure that when such a version is selected
		// we'll translate it back to the expected tag name to fetch the manifests.
		// This doesn't exercise the _full_ installation because most of it would be
		// redundant with the "happy path" test, but we fetch the package meta here
		// because that's the step that would need to resolve the tag.
		testVersion := MustParseVersion("1.0.2+foo.1")
		meta, err := source.PackageMeta(t.Context(), addrs.MustParseProviderSourceString("example.com/foo/bar"), testVersion, fakePlatforms[0])
		if err != nil {
			t.Fatal(err)
		}
		if got, want := meta.Version, testVersion; got != want {
			t.Errorf("wrong version\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run("provider that the source can't handle", func(t *testing.T) {
		// The resolveOCIRepositoryAddr function is set up to return ErrProviderNotFound when
		// asked for a provider that isn't on example.com, and so this test verifies that
		// this error type propagates correctly through AvailableVersions as [MultiSource]
		// expects so that it can disregard sources that don't know how to handle a
		// particular provider.
		unsupportedProvider := addrs.MustParseProviderSourceString("example.org/unsupported/domain")
		_, _, err := source.AvailableVersions(t.Context(), unsupportedProvider)
		gotErr, ok := err.(ErrProviderNotFound)
		if !ok {
			t.Fatalf("wrong error type\ngot:  %T (%s)\nwant: %T", err, err, gotErr)
		}
	})
	t.Run("provider that the source could handle but doesn't exist", func(t *testing.T) {
		// This is similar to the previous case but represents the dynamic form of the
		// problem, where the translation from provider address to OCI repository address
		// succeeded but then there is not actually an OCI repository at that address.
		unsupportedProvider := addrs.MustParseProviderSourceString("example.com/nonexist/foo")
		_, _, err := source.AvailableVersions(t.Context(), unsupportedProvider)
		gotErr, ok := err.(ErrProviderNotFound)
		if !ok {
			t.Fatalf("wrong error type\ngot:  %T (%s)\nwant: %T", err, err, gotErr)
		}
	})
	t.Run("request for unsupported platform", func(t *testing.T) {
		fakeProvider := addrs.MustParseProviderSourceString("example.com/foo/bar")
		_, err := source.PackageMeta(t.Context(), fakeProvider, MustParseVersion("1.0.0"), Platform{OS: "riscovite", Arch: "riscv64"})
		gotErr, ok := err.(ErrPlatformNotSupported)
		if !ok {
			t.Fatalf("wrong error type\ngot:  %T (%s)\nwant: %T", err, err, gotErr)
		}
	})
	t.Run("module package instead of provider", func(t *testing.T) {
		fakeProvider := addrs.MustParseProviderSourceString("example.com/not/provider")
		_, err := source.PackageMeta(t.Context(), fakeProvider, wrongStoreLookups.modulePackageVersion, fakePlatforms[0])
		if err == nil {
			t.Fatal("unexpected success; want error")
		}
		if got, want := err.Error(), `selected OCI artifact is an OpenTofu module package, not a provider package`; got != want {
			t.Errorf("wrong error\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run("container image instead of provider", func(t *testing.T) {
		fakeProvider := addrs.MustParseProviderSourceString("example.com/not/provider")
		_, err := source.PackageMeta(t.Context(), fakeProvider, wrongStoreLookups.containerImageVersion, fakePlatforms[0])
		if err == nil {
			t.Fatal("unexpected success; want error")
		}
		if got, want := err.Error(), `unsupported OCI artifact type; is this a container image, rather than an OpenTofu provider?`; got != want {
			t.Errorf("wrong error\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run("helm chart instead of provider", func(t *testing.T) {
		fakeProvider := addrs.MustParseProviderSourceString("example.com/not/provider")
		_, err := source.PackageMeta(t.Context(), fakeProvider, wrongStoreLookups.helmChartVersion, fakePlatforms[0])
		if err == nil {
			t.Fatal("unexpected success; want error")
		}
		// Note that the answer to the in the error message is "no" in this case:
		// it's not an OpenTofu provider, but it's not a container image either.
		// We can't differentiate these cases using only the information available
		// in the manifest descriptor, so we assume that accidentally selecting
		// a container image is more likely than accidentally selecting a Helm
		// chart, but phrase it as a question to hopefully communicate that
		// OpenTofu isn't actually sure what the artifact type is.
		if got, want := err.Error(), `unsupported OCI artifact type; is this a container image, rather than an OpenTofu provider?`; got != want {
			t.Errorf("wrong error\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run("tag directly refers to image manifest, not index manifest", func(t *testing.T) {
		fakeProvider := addrs.MustParseProviderSourceString("example.com/not/provider")
		_, err := source.PackageMeta(t.Context(), fakeProvider, wrongStoreLookups.missingIndexVersion, fakePlatforms[0])
		if err == nil {
			t.Fatal("unexpected success; want error")
		}
		if got, want := err.Error(), `tag refers directly to image manifest, but OpenTofu providers require an index manifest for multi-platform support`; got != want {
			t.Errorf("wrong error\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run("valid manifests but unsupported archive format", func(t *testing.T) {
		fakeProvider := addrs.MustParseProviderSourceString("example.com/not/provider")
		_, err := source.PackageMeta(t.Context(), fakeProvider, wrongStoreLookups.wrongPackageMediaTypeVersion, Platform{OS: "acornmos", Arch: "6502"})
		if err == nil {
			t.Fatal("unexpected success; want error")
		}
		if got, want := err.Error(), `image manifest contains no "application/vnd.opentofu.providerpkg" layers of type "archive/zip", but has other unsupported formats; this OCI artifact might be intended for a different version of OpenTofu`; got != want {
			t.Errorf("wrong error\ngot:  %s\nwant: %s", got, want)
		}
	})
}

func pushOCIImageManifest(t *testing.T, manifest *ociv1.Manifest, store orasContent.Pusher) ociv1.Descriptor {
	t.Helper()
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return pushOCIBlob(t, manifest.MediaType, manifest.ArtifactType, manifestBytes, store)
}

func pushOCIIndexManifest(t *testing.T, manifest *ociv1.Index, store orasContent.Pusher) ociv1.Descriptor {
	t.Helper()
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return pushOCIBlob(t, manifest.MediaType, manifest.ArtifactType, manifestBytes, store)
}

func pushOCIBlob(t *testing.T, mediaType, artifactType string, content []byte, store orasContent.Pusher) ociv1.Descriptor {
	t.Helper()
	desc := ociv1.Descriptor{
		Digest:       ociDigest.FromBytes(content),
		Size:         int64(len(content)),
		MediaType:    mediaType,
		ArtifactType: artifactType,
	}
	err := store.Push(t.Context(), desc, bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	return desc
}

func createOCITag(t *testing.T, tagName string, desc ociv1.Descriptor, store orasContent.Tagger) {
	t.Helper()
	err := store.Tag(t.Context(), desc, tagName)
	if err != nil {
		t.Fatal(err)
	}
}

// fakeOCIRepositoryContent is returned by [makeFakeOCIRepositoryWithNonProviderContent]
// to help the caller find each of the "fake" artifacts in the repository without having
// to hard-code the arbitrary version numbers used for each one. We use version numbers
// for these just because that's what [Source.PackageMeta] uses as its input, and so
// non-version-shaped tags are not accessible through our provider source API at all.
type fakeOCIRepositoryContent struct {
	modulePackageVersion         Version
	containerImageVersion        Version
	helmChartVersion             Version
	missingIndexVersion          Version
	wrongPackageMediaTypeVersion Version
}

func makeFakeOCIRepositoryWithNonProviderContent(t *testing.T) (OCIRepositoryStore, *fakeOCIRepositoryContent) {
	t.Helper()
	store, err := orasOCI.NewWithContext(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	lookups := &fakeOCIRepositoryContent{
		modulePackageVersion:         MustParseVersion("0.0.1"),
		containerImageVersion:        MustParseVersion("0.0.2"),
		helmChartVersion:             MustParseVersion("0.0.3"),
		missingIndexVersion:          MustParseVersion("0.0.4"),
		wrongPackageMediaTypeVersion: MustParseVersion("0.0.5"),
	}

	// This particular repository contains an assortment of odd manifests
	// that don't match OpenTofu's provider package manifest layout, so that
	// we can test that we generate plausible feedback when someone mistakenly
	// tries to use such things as providers. Some of these are based on
	// formats used by other OpenTofu features, some on other software that
	// makes use of OCI repositories, and some that's just strange garbage
	// that no software is likely to accept.
	//
	// Since the use of this is focused primarily on invalid metadata rather
	// than invalid data, we'll just use a single small blob as a placeholder
	// leaf object across all of the examples that need such a thing.
	blobDesc := pushOCIBlob(t, "application/octet-stream", "", []byte(`!`), store)

	// Many of the manifests also refer to the "well-known" empty JSON blob
	// digest, so we'll put that in the store too. This is a conventional way
	// to represent an empty JSON object so that each registry only needs to
	// store it once.
	pushOCIBlob(
		t,
		ociv1.DescriptorEmptyJSON.MediaType,
		ociv1.DescriptorEmptyJSON.ArtifactType,
		ociv1.DescriptorEmptyJSON.Data,
		store,
	)

	// The following nested blocks are just to keep the temporary variables
	// segregated for each case to avoid any risk of getting things mixed up
	// under future maintenence.
	{
		// a manifest that follows our conventions for OpenTofu module packages, rather
		// than for provider packages.
		archiveDesc := blobDesc // shallow copy
		archiveDesc.ArtifactType = "application/vnd.opentofu.modulepkg"
		archiveDesc.MediaType = "archive/zip"
		manifestDesc := pushOCIImageManifest(t, &ociv1.Manifest{
			Versioned:    ociSpecs.Versioned{SchemaVersion: 2},
			MediaType:    ociv1.MediaTypeImageManifest,
			ArtifactType: archiveDesc.ArtifactType,
			Config:       ociv1.DescriptorEmptyJSON,
			Layers: []ociv1.Descriptor{
				archiveDesc,
			},
		}, store)
		createOCITag(t, lookups.modulePackageVersion.String(), manifestDesc, store)
	}
	{
		// a manifest that resembles a container image
		archiveDesc := blobDesc       // shallow copy
		archiveDesc.ArtifactType = "" // container images are essentially the "default" artifact type
		archiveDesc.MediaType = ociv1.MediaTypeImageLayerGzip
		manifestDesc := pushOCIImageManifest(t, &ociv1.Manifest{
			Versioned: ociSpecs.Versioned{SchemaVersion: 2},
			MediaType: ociv1.MediaTypeImageManifest,
			Config:    ociv1.DescriptorEmptyJSON,
			Layers: []ociv1.Descriptor{
				archiveDesc,
			},
		}, store)
		createOCITag(t, lookups.containerImageVersion.String(), manifestDesc, store)
	}
	{
		// a manifest that resembles a Helm chart
		chartPkgDesc := blobDesc       // shallow copy
		chartPkgDesc.ArtifactType = "" // Helm uses a custom media type rather instead of an artifact type
		chartPkgDesc.MediaType = "application/vnd.cncf.helm.chart.content.v1.tar+gzip"
		manifestDesc := pushOCIImageManifest(t, &ociv1.Manifest{
			Versioned: ociSpecs.Versioned{SchemaVersion: 2},
			MediaType: ociv1.MediaTypeImageManifest,
			Config: ociv1.Descriptor{
				MediaType: "application/vnd.cncf.helm.chart.content.v1.tar+gzip",
				Digest:    chartPkgDesc.Digest,
				Size:      chartPkgDesc.Size,
			},
			Layers: []ociv1.Descriptor{
				chartPkgDesc,
			},
		}, store)
		createOCITag(t, lookups.helmChartVersion.String(), manifestDesc, store)
	}
	{
		// a manifest for a single provider package lacking the required index manifest
		archiveDesc := blobDesc // shallow copy
		archiveDesc.ArtifactType = ociPackageArtifactType
		archiveDesc.MediaType = ociPackageMediaType
		manifestDesc := pushOCIImageManifest(t, &ociv1.Manifest{
			Versioned:    ociSpecs.Versioned{SchemaVersion: 2},
			MediaType:    ociv1.MediaTypeImageManifest,
			ArtifactType: ociPackageManifestArtifactType,
			Config:       ociv1.DescriptorEmptyJSON,
			Layers: []ociv1.Descriptor{
				archiveDesc,
			},
		}, store)
		createOCITag(t, lookups.missingIndexVersion.String(), manifestDesc, store)
	}
	{
		// a manifest for a provider that is valid except for using an as-yet-unsupported
		// archive format for the leaf provider package.
		archiveDesc := blobDesc                                // shallow copy
		archiveDesc.ArtifactType = ociPackageArtifactType      // correct artifact type
		archiveDesc.MediaType = "application/x-lzh-compressed" // unsupported media type
		manifestDesc := pushOCIImageManifest(t, &ociv1.Manifest{
			Versioned:    ociSpecs.Versioned{SchemaVersion: 2},
			MediaType:    ociv1.MediaTypeImageManifest,
			ArtifactType: ociPackageManifestArtifactType,
			Config:       ociv1.DescriptorEmptyJSON,
			Layers: []ociv1.Descriptor{
				archiveDesc,
			},
		}, store)
		manifestDesc.Platform = &ociv1.Platform{
			Architecture: "6502",
			OS:           "acornmos",
		}
		indexDesc := pushOCIIndexManifest(t, &ociv1.Index{
			Versioned:    ociSpecs.Versioned{SchemaVersion: 2},
			MediaType:    ociv1.MediaTypeImageIndex,
			ArtifactType: ociIndexManifestArtifactType,
			Manifests: []ociv1.Descriptor{
				manifestDesc,
			},
		}, store)
		createOCITag(t, lookups.wrongPackageMediaTypeVersion.String(), indexDesc, store)
	}

	return store, lookups
}
