// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/apparentlymart/go-versions/versions"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"

	"github.com/placeholderplaceholderplaceholder/opentf/internal/addrs"
)

// OCISource is a Source that knows how to find and install providers from
// their originating provider registries.
type OCISource struct {
}

var _ Source = (*OCISource)(nil)

// NewOCISource creates and returns a new source that will install
// providers from their originating provider registries.
func NewOCISource() *OCISource {
	return &OCISource{}
}

// AvailableVersions returns all of the versions available for the provider
// with the given address, or an error if that result cannot be determined.
//
// If the request fails, the returned error might be an value of
// ErrHostNoProviders, ErrHostUnreachable, ErrUnauthenticated,
// ErrProviderNotKnown, or ErrQueryFailed. Callers must be defensive and
// expect errors of other types too, to allow for future expansion.
func (s *OCISource) AvailableVersions(ctx context.Context, provider addrs.Provider) (VersionList, Warnings, error) {
	repo, err := remote.NewRepository(fmt.Sprintf("%s/%s/%s", provider.Hostname, provider.Namespace, provider.Type))
	if err != nil {
		panic(err)
	}
	repo.PlainHTTP = true

	var out VersionList
	if err := repo.Tags(ctx, "", func(tags []string) error {
		for _, tag := range tags {
			v, err := versions.ParseVersion(tag)
			if err == nil {
				out = append(out, v)
			}
		}
		return nil
	}); err != nil {
		panic(err)
	}

	return out, nil, nil
}

// PackageMeta returns metadata about the location and capabilities of
// a distribution package for a particular provider at a particular version
// targeting a particular platform.
//
// Callers of PackageMeta should first call AvailableVersions and pass
// one of the resulting versions to this function. This function cannot
// distinguish between a version that is not available and an unsupported
// target platform, so if it encounters either case it will return an error
// suggesting that the target platform isn't supported under the assumption
// that the caller already checked that the version is available at all.
//
// To find a package suitable for the platform where the provider installation
// process is running, set the "target" argument to
// getproviders.CurrentPlatform.
//
// If the request fails, the returned error might be an value of
// ErrHostNoProviders, ErrHostUnreachable, ErrUnauthenticated,
// ErrPlatformNotSupported, or ErrQueryFailed. Callers must be defensive and
// expect errors of other types too, to allow for future expansion.
func (s *OCISource) PackageMeta(ctx context.Context, provider addrs.Provider, version Version, target Platform) (PackageMeta, error) {
	// TODO: Host metadata as blob or annotations.
	repo, err := remote.NewRepository(fmt.Sprintf("%s/%s/%s", provider.Hostname, provider.Namespace, provider.Type))
	if err != nil {
		panic(err)
	}
	repo.PlainHTTP = true

	_, packageDescBytes, err := oras.FetchBytes(ctx, repo, version.String(), oras.FetchBytesOptions{
		FetchOptions: oras.FetchOptions{
			ResolveOptions: oras.ResolveOptions{
				TargetPlatform: &ocispec.Platform{
					Architecture: runtime.GOARCH,
					OS:           runtime.GOOS,
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	var packageDesc ocispec.Manifest
	if err := json.Unmarshal(packageDescBytes, &packageDesc); err != nil {
		panic(err)
	}

	meta := PackageMeta{
		Provider:       provider,
		Version:        version,
		TargetPlatform: target,

		// Because this is already unpacked, the filename is synthetic
		// based on the standard naming scheme.
		Filename: fmt.Sprintf("terraform-provider-%s_%s_%s.zip", provider.Type, version, target),
		Location: PackageOCIBlob{
			Repository: fmt.Sprintf("%s/%s/%s", provider.Hostname, provider.Namespace, provider.Type),
			Digest:     packageDesc.Layers[0].Digest.String(),
		},
	}

	return meta, nil
}

func (s *OCISource) ForDisplay(provider addrs.Provider) string {
	return fmt.Sprintf("registry %s", provider.Hostname.ForDisplay())
}
