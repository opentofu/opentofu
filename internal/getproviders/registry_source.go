// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/opentofu/svchost"
	disco "github.com/opentofu/svchost/disco"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/httpclient"
)

// RegistrySource is a Source that knows how to find and install providers from
// their originating provider registries.
type RegistrySource struct {
	services   *disco.Disco
	httpClient *retryablehttp.Client
}

var _ Source = (*RegistrySource)(nil)

// NewRegistrySource creates and returns a new source that will install
// providers from their originating provider registries.
func NewRegistrySource(ctx context.Context, services *disco.Disco, httpClient *retryablehttp.Client) *RegistrySource {
	if httpClient == nil {
		// As an aid to our tests that don't really care that much about
		// the HTTP client configuration, we'll provide some reasonable
		// defaults if no custom client is provided.
		httpClient = httpclient.NewForRegistryRequests(ctx, 1, 10*time.Second)
	}

	return &RegistrySource{
		services:   services,
		httpClient: httpClient,
	}
}

// AvailableVersions returns all of the versions available for the provider
// with the given address, or an error if that result cannot be determined.
//
// If the request fails, the returned error might be an value of
// ErrHostNoProviders, ErrHostUnreachable, ErrUnauthenticated,
// ErrProviderNotKnown, or ErrQueryFailed. Callers must be defensive and
// expect errors of other types too, to allow for future expansion.
func (s *RegistrySource) AvailableVersions(ctx context.Context, provider addrs.Provider) (VersionList, Warnings, error) {
	client, err := s.registryClient(ctx, provider.Hostname)
	if err != nil {
		return nil, nil, err
	}

	versionsResponse, warnings, err := client.ProviderVersions(ctx, provider)
	if err != nil {
		return nil, nil, err
	}

	if len(versionsResponse) == 0 {
		return nil, warnings, nil
	}

	// We ignore protocols here because our goal is to find out which versions
	// are available _at all_. Which ones are compatible with the current
	// OpenTofu becomes relevant only once we've selected one, at which point
	// we'll return an error if the selected one is incompatible.
	//
	// We intentionally produce an error on incompatibility, rather than
	// silently ignoring an incompatible version, in order to give the user
	// explicit feedback about why their selection wasn't valid and allow them
	// to decide whether to fix that by changing the selection or by some other
	// action such as upgrading OpenTofu, using a different OS to run
	// OpenTofu, etc. Changes that affect compatibility are considered breaking
	// changes from a provider API standpoint, so provider teams should change
	// compatibility only in new major versions.
	ret := make(VersionList, 0, len(versionsResponse))
	for str := range versionsResponse {
		v, err := ParseVersion(str)
		if err != nil {
			return nil, nil, ErrQueryFailed{
				Provider: provider,
				Wrapped:  fmt.Errorf("registry response includes invalid version string %q: %w", str, err),
			}
		}
		ret = append(ret, v)
	}
	ret.Sort() // lowest precedence first, preserving order when equal precedence
	return ret, warnings, nil
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
func (s *RegistrySource) PackageMeta(ctx context.Context, provider addrs.Provider, version Version, target Platform) (PackageMeta, error) {
	client, err := s.registryClient(ctx, provider.Hostname)
	if err != nil {
		return PackageMeta{}, err
	}

	return client.PackageMeta(ctx, provider, version, target)
}

func (s *RegistrySource) registryClient(ctx context.Context, hostname svchost.Hostname) (*registryClient, error) {
	host, err := s.services.Discover(ctx, hostname)
	if err != nil {
		return nil, ErrHostUnreachable{
			Hostname: hostname,
			Wrapped:  err,
		}
	}

	url, err := host.ServiceURL("providers.v1")
	switch err := err.(type) {
	case nil:
		// okay! We'll fall through and return below.
	case *disco.ErrServiceNotProvided:
		return nil, ErrHostNoProviders{
			Hostname: hostname,
		}
	case *disco.ErrVersionNotSupported:
		return nil, ErrHostNoProviders{
			Hostname:        hostname,
			HasOtherVersion: true,
		}
	default:
		return nil, ErrHostUnreachable{
			Hostname: hostname,
			Wrapped:  err,
		}
	}

	// Check if we have credentials configured for this hostname.
	creds, err := s.services.CredentialsForHost(ctx, hostname)
	if err != nil {
		// This indicates that a credentials helper failed, which means we
		// can't do anything better than just pass through the helper's
		// own error message.
		return nil, fmt.Errorf("failed to retrieve credentials for %s: %w", hostname, err)
	}

	return newRegistryClient(ctx, url, creds, s.httpClient), nil
}

func (s *RegistrySource) ForDisplay(provider addrs.Provider) string {
	return fmt.Sprintf("registry %s", provider.Hostname.ForDisplay())
}
