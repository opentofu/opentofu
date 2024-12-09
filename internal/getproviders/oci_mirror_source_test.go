// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"fmt"
	"testing"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/google/go-cmp/cmp"
	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/opentofu/libregistry/registryprotocols/ociclient"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestOCIMirrorSource(t *testing.T) {
	// This is just a stub test for now, since the source itself is just a stub.
	// We'll replace this with a more complete test once we have a more complete implementation!

	fakeClient := &fakeOCIClient{
		listReferencesFunc: func(addr ociclient.OCIAddr) ([]ociclient.OCIReference, ociclient.OCIWarnings, error) {
			if string(addr.Registry) != "example.net" {
				return nil, nil, fmt.Errorf("wrong registry hostname %q", addr.Registry)
			}
			if string(addr.Name) != "example.com/foo/bar" {
				return nil, nil, fmt.Errorf("wrong repository name %q", addr.Name)
			}
			return []ociclient.OCIReference{
				ociclient.OCIReference("1.0.0"),
				ociclient.OCIReference("ignored-nonversion"),
			}, nil, nil
		},
		resolvePlatformImageDigestFunc: func(addr ociclient.OCIAddrWithReference, config *ociclient.ClientPullConfig) (ociclient.OCIDigest, ociclient.OCIWarnings, error) {
			if string(addr.Registry) != "example.net" {
				return ociclient.OCIDigest(""), nil, fmt.Errorf("wrong registry hostname %q", addr.Registry)
			}
			if string(addr.Name) != "example.com/foo/bar" {
				return ociclient.OCIDigest(""), nil, fmt.Errorf("wrong repository name %q", addr.Name)
			}
			if config.GOOS != "msdos" {
				return ociclient.OCIDigest(""), nil, fmt.Errorf("wrong config.GOOS %q", config.GOOS)
			}
			if config.GOARCH != "286" {
				return ociclient.OCIDigest(""), nil, fmt.Errorf("wrong config.GOARCH %q", config.GOARCH)
			}
			return ociclient.OCIDigest("sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"), nil, nil
		},
	}
	source := NewOCIMirrorSource(fakeClient, func(providerAddr addrs.Provider) (OCIRepository, tfdiags.Diagnostics) {
		return OCIRepository{
			Hostname: "example.net",
			Name:     fmt.Sprintf("%s/%s/%s", providerAddr.Hostname, providerAddr.Namespace, providerAddr.Type),
		}, nil
	})

	ctx := context.Background()
	providerAddr := addrs.Provider{
		Hostname:  svchost.Hostname("example.com"),
		Namespace: "foo",
		Type:      "bar",
	}
	platform := Platform{
		OS:   "msdos",
		Arch: "286",
	}

	wantVersions := versions.List{
		versions.MustParseVersion("1.0.0"),
	}
	gotVersions, warnings, err := source.AvailableVersions(ctx, providerAddr)
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %#v", warnings)
	}
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if diff := cmp.Diff(wantVersions, gotVersions); diff != "" {
		t.Fatal("wrong AvailableVersions result:\n" + diff)
	}

	meta, err := source.PackageMeta(ctx, providerAddr, wantVersions[0], platform)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	location, ok := meta.Location.(PackageOCIObject)
	if !ok {
		t.Fatalf("PackageMeta.Location is %T, but want %T", meta.Location, location)
	}
	if got, want := location.repositoryAddr.String(), "example.net/example.com/foo/bar"; got != want {
		t.Errorf("wrong repository\ngot:  %s\nwant: %s", got, want)
	}
	if got, want := string(location.imageManifestDigest), "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"; got != want {
		t.Errorf("wrong manifest digest\ngot:  %s\nwant: %s", got, want)
	}
}

func TestOCIMirrorSource_unmappableProvider(t *testing.T) {
	// The OpenTofu provider source address syntax permits a wider repertiore
	// of unicode characters than the OCI distribution spec allows in its
	// names, so some provider addresses cannot currently be mirrored
	// in an OCI registry unless they are matched exactly and translated
	// to a name that's not mechanically derived from the source address.

	fakeClient := &fakeOCIClient{
		// No implementations required because this test wants calls to fail
		// before interacting with the client.
	}
	source := NewOCIMirrorSource(fakeClient, func(providerAddr addrs.Provider) (OCIRepository, tfdiags.Diagnostics) {
		return OCIRepository{
			Hostname: "example.net",
			Name:     fmt.Sprintf("%s/terraform-provider-%s", providerAddr.Namespace, providerAddr.Type),
		}, nil
	})

	ctx := context.Background()
	providerAddr := addrs.Provider{
		Hostname:  svchost.Hostname("example.com"),
		Namespace: "ほげ",
		Type:      "ふが",
	}
	platform := Platform{
		OS:   "human68k",
		Arch: "68000",
	}
	_, _, err := source.AvailableVersions(ctx, providerAddr)
	if got, want := err.Error(), `requested provider address "example.com/ほげ/ふが" contains characters that are not valid in an OCI distribution repository name, so this provider cannot be installed from an OCI repository as "ほげ/terraform-provider-ふが"`; got != want {
		t.Errorf("wrong error from AvailableVersions\ngot:  %s\nwant: %s", got, want)
	}
	_, err = source.PackageMeta(ctx, providerAddr, versions.MustParseVersion("1.0.0"), platform)
	if got, want := err.Error(), `requested provider address "example.com/ほげ/ふが" contains characters that are not valid in an OCI distribution repository name, so this provider cannot be installed from an OCI repository as "ほげ/terraform-provider-ふが"`; got != want {
		t.Errorf("wrong error from PackageMeta\ngot:  %s\nwant: %s", got, want)
	}
}

type fakeOCIClient struct {
	listReferencesFunc             func(addr ociclient.OCIAddr) ([]ociclient.OCIReference, ociclient.OCIWarnings, error)
	resolvePlatformImageDigestFunc func(addr ociclient.OCIAddrWithReference, config *ociclient.ClientPullConfig) (ociclient.OCIDigest, ociclient.OCIWarnings, error)
	pullImageWithImageDigestFunc   func(addrRef ociclient.OCIAddrWithDigest) (ociclient.PulledOCIImage, ociclient.OCIWarnings, error)
	pullImageFunc                  func(addr ociclient.OCIAddrWithReference, config *ociclient.ClientPullConfig) (ociclient.PulledOCIImage, ociclient.OCIWarnings, error)
}

var _ ociclient.OCIClient = (*fakeOCIClient)(nil)

// ListReferences implements ociclient.OCIClient.
func (f *fakeOCIClient) ListReferences(_ context.Context, addr ociclient.OCIAddr) ([]ociclient.OCIReference, ociclient.OCIWarnings, error) {
	if f.listReferencesFunc == nil {
		return nil, nil, fmt.Errorf("fake OCI distribution client does not implement ListReferences")
	}
	return f.listReferencesFunc(addr)
}

// ResolvePlatformImageDigest implements ociclient.OCIClient.
func (f *fakeOCIClient) ResolvePlatformImageDigest(_ context.Context, addr ociclient.OCIAddrWithReference, opts ...ociclient.ClientPullOpt) (ociclient.OCIDigest, ociclient.OCIWarnings, error) {
	if f.resolvePlatformImageDigestFunc == nil {
		return ociclient.OCIDigest(""), nil, fmt.Errorf("fake OCI distribution client does not implement ResolvePlatformImageDigest")
	}
	config := &ociclient.ClientPullConfig{}
	for _, opt := range opts {
		err := opt(config)
		if err != nil {
			return ociclient.OCIDigest(""), nil, fmt.Errorf("configuration error: %w", err)
		}
	}
	return f.resolvePlatformImageDigestFunc(addr, config)
}

// PullImageWithImageDigest implements ociclient.OCIClient.
func (f *fakeOCIClient) PullImageWithImageDigest(_ context.Context, addrRef ociclient.OCIAddrWithDigest) (ociclient.PulledOCIImage, ociclient.OCIWarnings, error) {
	if f.pullImageWithImageDigestFunc == nil {
		return nil, nil, fmt.Errorf("fake OCI distribution client does not implement PullImageWithImageDigest")
	}
	return f.pullImageWithImageDigestFunc(addrRef)
}

// PullImage implements ociclient.OCIClient.
func (f *fakeOCIClient) PullImage(_ context.Context, addr ociclient.OCIAddrWithReference, opts ...ociclient.ClientPullOpt) (ociclient.PulledOCIImage, ociclient.OCIWarnings, error) {
	if f.pullImageFunc == nil {
		return nil, nil, fmt.Errorf("fake OCI distribution client does not implement PullImage")
	}
	config := &ociclient.ClientPullConfig{}
	for _, opt := range opts {
		err := opt(config)
		if err != nil {
			return nil, nil, fmt.Errorf("configuration error: %w", err)
		}
	}
	return f.pullImageFunc(addr, config)
}
