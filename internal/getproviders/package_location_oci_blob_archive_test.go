// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/google/go-cmp/cmp"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	orasContent "oras.land/oras-go/v2/content"
	orasMemoryStore "oras.land/oras-go/v2/content/memory"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/collections"
)

func TestPackageOCIBlobArchive(t *testing.T) {
	// To avoid any external dependencies we'll implement this test in terms
	// of an in-memory-only OCI repository. In real use we instead use the
	// ORAS-Go remote registry client, which implements the same interfaces.
	//
	// [PackageOCIBlobArchive] focuses only on the leaf archive artifact,
	// so we don't need to include any manifests here as they would normally
	// be dealt with by the [Source] implementation instead.
	store := orasMemoryStore.New()

	// Unfortunately we need to first construct our blob in a separate RAM
	// buffer here because we need to calculate a checksum for it in order
	// to "push" it into the store.
	blobBytes := makePlaceholderProviderPackageZip(t, "not a real executable; just a placeholder")
	desc := pushOCIBlob(t, "archive/zip", "", blobBytes, store)

	t.Run("happy path", func(t *testing.T) {
		// The in-memory OCI repository contains just the blob represented
		// by desc, which would not be enough to query the package metadata
		// through the [Source] API, but is sufficient for the limited scope
		// of [PackageOCIBlobArchive], since that is interested only in the
		// archive blob and assumes that some other component already
		// used tags and manifests to select a suitable blob.
		loc := PackageOCIBlobArchive{
			repoStore:      store,
			blobDescriptor: desc,
		}
		meta := PackageMeta{
			Provider:       addrs.NewBuiltInProvider("foo"),
			Version:        versions.MustParseVersion("1.0.0"),
			TargetPlatform: CurrentPlatform,
			Location:       loc,
			Authentication: &mockAuthentication{
				hashes: HashDispositions{
					Hash("test:placeholder"): {
						SignedByGPGKeyIDs: collections.NewSet("abc123"),
					},
				},
			},
		}
		targetDir := t.TempDir()
		authResult, err := loc.InstallProviderPackage(t.Context(), meta, targetDir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		// Installation was successful, so we should now have the two files guaranteed
		// by the documentation of [makePlaceholderProviderPackageZip].
		if diff := diffDirForPlaceholderProviderPackageZip(t, targetDir); diff != "" {
			t.Error("wrong directory contents after successful installation\n" + diff)
		}
		if !authResult.Signed() {
			// This fake package isn't _actually_ signed, but the mockAuthentication
			// object we passed in meta reports that it was and so that information
			// should pass through to the installation result if the installation
			// process is correctly implemented.
			t.Errorf("auth result does not indicate that the package is signed")
		}
	})
	t.Run("checksum mismatch", func(t *testing.T) {
		loc := PackageOCIBlobArchive{
			repoStore:      store,
			blobDescriptor: desc,
		}
		meta := PackageMeta{
			Provider:       addrs.NewBuiltInProvider("foo"),
			Version:        versions.MustParseVersion("1.0.0"),
			TargetPlatform: CurrentPlatform,
			Location:       loc,
		}
		targetDir := t.TempDir()
		_, err := loc.InstallProviderPackage(t.Context(), meta, targetDir, []Hash{
			HashSchemeZip.New("not-valid-never-matches"),
		})
		const wantErrSubstr = `doesn't match any of the checksums`
		if err == nil {
			t.Fatalf("unexpected success\nwant error containing: %s", wantErrSubstr)
		}
		if gotErr := err.Error(); !strings.Contains(gotErr, wantErrSubstr) {
			t.Fatalf("wrong error\ngot: %s\nwant substring: %s", gotErr, wantErrSubstr)
		}
		// The checksum failure should've been detected before actually
		// extracting the package.
		if diff := diffDirEmpty(t, targetDir); diff != "" {
			t.Error("unexpected content in target directory\n" + diff)
		}
	})
	t.Run("unsupported archive type", func(t *testing.T) {
		wrongDesc := desc                                    // shallow copy
		wrongDesc.MediaType = "application/x-lzh-compressed" // an unsupported archive type
		loc := PackageOCIBlobArchive{
			repoStore:      store,
			blobDescriptor: wrongDesc,
		}
		meta := PackageMeta{
			Provider:       addrs.NewBuiltInProvider("foo"),
			Version:        versions.MustParseVersion("1.0.0"),
			TargetPlatform: CurrentPlatform,
			Location:       loc,
		}
		targetDir := t.TempDir()
		_, err := loc.InstallProviderPackage(t.Context(), meta, targetDir, nil)
		const wantErrSubstr = `selected OCI artifact manifest has unexpected media type "application/x-lzh-compressed"`
		if err == nil {
			t.Fatalf("unexpected success\nwant error containing: %s", wantErrSubstr)
		}
		if gotErr := err.Error(); !strings.Contains(gotErr, wantErrSubstr) {
			t.Fatalf("wrong error\ngot: %s\nwant substring: %s", gotErr, wantErrSubstr)
		}
		// The unsupported format should've been detected before actually
		// extracting the package. (The blob we provided was actually a
		// zip archive despite the incorrect media type, so it could
		// potentially still be extracted despite the media type problem.
		if diff := diffDirEmpty(t, targetDir); diff != "" {
			t.Error("unexpected content in target directory\n" + diff)
		}
	})
	t.Run("descriptor has incorrect size", func(t *testing.T) {
		wrongDesc := desc  // shallow copy
		wrongDesc.Size = 4 // not the actual size of the zip archive
		loc := PackageOCIBlobArchive{
			repoStore:      store,
			blobDescriptor: wrongDesc,
		}
		meta := PackageMeta{
			Provider:       addrs.NewBuiltInProvider("foo"),
			Version:        versions.MustParseVersion("1.0.0"),
			TargetPlatform: CurrentPlatform,
			Location:       loc,
		}
		targetDir := t.TempDir()
		_, err := loc.InstallProviderPackage(t.Context(), meta, targetDir, nil)
		if err == nil {
			// Unfortunately the shape of this error is not guaranteed since each
			// implementation of orasContent.ReadOnlyStorage detects and reports
			// this problem in a different way, so we can only test whether it failed.
			t.Fatalf("unexpected success; want some sort of error about the descriptor being incorrect")
		}
	})
	t.Run("descriptor has digest referring to missing blob", func(t *testing.T) {
		wrongDesc := desc                                   // shallow copy
		wrongDesc.Digest = ociv1.DescriptorEmptyJSON.Digest // not actually present in our store
		loc := PackageOCIBlobArchive{
			repoStore:      store,
			blobDescriptor: wrongDesc,
		}
		meta := PackageMeta{
			Provider:       addrs.NewBuiltInProvider("foo"),
			Version:        versions.MustParseVersion("1.0.0"),
			TargetPlatform: CurrentPlatform,
			Location:       loc,
		}
		targetDir := t.TempDir()
		_, err := loc.InstallProviderPackage(t.Context(), meta, targetDir, nil)
		if err == nil {
			// Unfortunately the shape of this error is not guaranteed since each
			// implementation of orasContent.ReadOnlyStorage detects and reports
			// this problem in a different way, so we can only test whether it failed.
			t.Fatalf("unexpected success; want some sort of error about the descriptor being incorrect")
		}
	})
	t.Run("misbehaving store returns wrong content", func(t *testing.T) {
		// In this case we use the correct descriptor, but use a special store
		// implementation that intentionally returns incorrect results.
		// The installation process should detect the problem by verifying
		// what it received against the digest in the checksum.
		loc := PackageOCIBlobArchive{
			// The "lying storage" will return a blob that is correctly-sized
			// but has the wrong content, to make sure that we're actually
			// checking the content against the digest.
			repoStore:      LyingORASStorage{wantSize: desc.Size},
			blobDescriptor: desc,
		}
		meta := PackageMeta{
			Provider:       addrs.NewBuiltInProvider("foo"),
			Version:        versions.MustParseVersion("1.0.0"),
			TargetPlatform: CurrentPlatform,
			Location:       loc,
		}
		targetDir := t.TempDir()
		_, err := loc.InstallProviderPackage(t.Context(), meta, targetDir, nil)
		const wantErrSubstr = `provider package does not match digest`
		if err == nil {
			t.Fatalf("unexpected success\nwant error containing: %s", wantErrSubstr)
		}
		if gotErr := err.Error(); !strings.Contains(gotErr, wantErrSubstr) {
			t.Fatalf("wrong error\ngot: %s\nwant substring: %s", gotErr, wantErrSubstr)
		}
	})
}

// makePlaceholderProviderPackageZip returns a byte array with data that
// would, if written to disk, represent a valid zip archive acting as
// a placeholder for a provider plugin package.
//
// The zip archive includes files named "terraform-provider-foo" and
// "README". The "terraform-provider-foo" file is generated with the
// content given in fakeExeContent, while the "README" content is
// unspecified since it's just here to act as additional baggage that
// a real provider could hypothetically make use of but OpenTofu itself
// doesn't care about.
func makePlaceholderProviderPackageZip(t *testing.T, fakeExeContent string) []byte {
	t.Helper()

	buf := bytes.NewBuffer(nil)
	zipW := zip.NewWriter(buf)
	exeW, err := zipW.Create("terraform-provider-foo")
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.WriteString(exeW, fakeExeContent)
	if err != nil {
		t.Fatal(err)
	}
	// additional file to make sure we get the entire package, and not just the executable
	docW, err := zipW.Create("README")
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.WriteString(docW, "not a real plugin; just a placeholder")
	if err != nil {
		t.Fatal(err)
	}
	err = zipW.Close()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// diffDirForPlaceholderProviderPackageZip returns a string representation
// of a diff between the entries in the given directory and the files
// placed into an archive returned by [makePlaceholderProviderPackageZip].
//
// If the result is an empty string then there are no differences and so the
// directory does appear to have been created from the content of such a
// zip file.
func diffDirForPlaceholderProviderPackageZip(t *testing.T, dir string) string {
	t.Helper()

	type DiffableEntry struct {
		Filename string
		IsDir    bool
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]DiffableEntry, len(entries))
	for i, entry := range entries {
		got[i] = DiffableEntry{
			Filename: entry.Name(),
			IsDir:    entry.IsDir(),
		}
	}
	return cmp.Diff([]DiffableEntry{
		{"README", false},
		{"terraform-provider-foo", false},
	}, got)
}

// diffDirForPlaceholderProviderPackageZip returns a string representation
// of a diff between the entries in the given directory and an empty
// directory.
//
// If the result is an empty string then there are no differences and so the
// directory is empty.
func diffDirEmpty(t *testing.T, dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(entries))
	for i, entry := range entries {
		got[i] = entry.Name()
	}
	return cmp.Diff([]string{}, got)
}

// LyingORASStorage is an implementation of [orasContent.ReadOnlyStorage]
// which always returns fixed content regardless of what descriptor it is
// given, which allows us to test that we're resilient against a remote
// OCI registry returning incorrect content for a blob.
type LyingORASStorage struct {
	wantSize int64
}

var _ orasContent.ReadOnlyStorage = LyingORASStorage{}

// Exists implements [orasContent.ReadOnlyStorage].
func (s LyingORASStorage) Exists(ctx context.Context, target ociv1.Descriptor) (bool, error) {
	// We pretend that everything exists
	return true, nil
}

// Fetch implements [orasContent.ReadOnlyStorage].
func (s LyingORASStorage) Fetch(ctx context.Context, target ociv1.Descriptor) (io.ReadCloser, error) {
	// We return a blob consisting of s.wantSize bytes of garbage, which
	// the caller should therefore check and find that it mismatches
	// the digest in the descriptor, unless the caller happens to ask
	// for a digest that matches the garbage, which is vanishingly unlikely.
	return io.NopCloser(io.LimitReader(rand.Reader, s.wantSize)), nil
}
