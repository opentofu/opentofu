// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"fmt"

	"github.com/apparentlymart/go-versions/versions"
	disco "github.com/hashicorp/terraform-svchost/disco"
	"github.com/opentofu/opentofu/internal/tfdiags"
	tfaddr "github.com/opentofu/registry-address"
)

// DirectSource is a Source that handles the "direct" installation method by
// performing service discovery on a provider's origin registry hostname and
// then using the declared services to decide how to handle the request.
type DirectSource struct {
	services *disco.Disco
}

var _ Source = (*DirectSource)(nil)

// NewRegistrySource creates and returns a new source that will install
// providers from their originating provider registries.
func NewDirectSource(services *disco.Disco) *DirectSource {
	return &DirectSource{
		services: services,
	}
}

// AvailableVersions implements Source.
func (s *DirectSource) AvailableVersions(ctx context.Context, provider tfaddr.Provider) (versions.List, []string, error) {
	realSource, err := s.discoverRealSource(ctx, provider)
	if err != nil {
		return nil, nil, err
	}
	return realSource.AvailableVersions(ctx, provider)
}

// PackageMeta implements Source.
func (s *DirectSource) PackageMeta(ctx context.Context, provider tfaddr.Provider, version versions.Version, target Platform) (PackageMeta, error) {
	realSource, err := s.discoverRealSource(ctx, provider)
	if err != nil {
		return PackageMeta{}, err
	}
	return realSource.PackageMeta(ctx, provider, version, target)
}

// ForDisplay implements Source.
func (s *DirectSource) ForDisplay(provider tfaddr.Provider) string {
	return fmt.Sprintf("registry %s", provider.Hostname.ForDisplay())
}

// discoverRealSource finds an appropriate implementation of [Source] to use
// for the given provider based on network service discovery, or returns
// an error if no suitable service is available.
func (s *DirectSource) discoverRealSource(ctx context.Context, provider tfaddr.Provider) (Source, error) {
	host, err := s.services.Discover(provider.Hostname)
	if err != nil {
		return nil, ErrHostUnreachable{
			Hostname: provider.Hostname,
			Wrapped:  err,
		}
	}

	// Our first preference is to use OpenTofu's own provider registry protocol.
	realSource, err := s.providerRegistrySource(ctx, provider, host)
	//nolint:errorlint // we are intentionally not matching wrapped errors here; we want only exactly this error
	if mainErr, ok := err.(ErrHostNoProviders); ok {
		// If the main provider registry protocol isn't supported then we'll try
		// using the OCI registry service type as a fallback.
		realSource, err = s.ociMirrorSource(ctx, provider, host)
		//nolint:errorlint // we are intentionally not matching wrapped errors here; we want only exactly this error
		if _, ok := err.(ErrHostNoProviders); ok {
			// We'll keep the error from providerRegistrySource instead since
			// that one might report that this host requires a newer version
			// of OpenTofu, if the host declares a newer version of "providers".
			err = mainErr
		}
	}
	if err != nil {
		return nil, err
	}

	return realSource, err
}

func (s *DirectSource) providerRegistrySource(_ context.Context, provider tfaddr.Provider, services *disco.Host) (Source, error) {
	_, err := services.ServiceURL("providers.v1")
	//nolint:errorlint // this is intentionally following the structure from RegistrySource, for now
	switch err := err.(type) {
	case nil:
		// Passing through the same "Disco" object that
		// we already used means that the service discovery
		// result will be cached and so we won't need another
		// round-trip for this source to find the actual
		// URL it should use.
		return NewRegistrySource(s.services), nil
	case *disco.ErrServiceNotProvided:
		return nil, ErrHostNoProviders{
			Hostname: provider.Hostname,
		}
	case *disco.ErrVersionNotSupported:
		return nil, ErrHostNoProviders{
			Hostname:        provider.Hostname,
			HasOtherVersion: true,
		}
	default:
		return nil, ErrHostUnreachable{
			Hostname: provider.Hostname,
			Wrapped:  err,
		}
	}
}

func (s *DirectSource) ociMirrorSource(_ context.Context, provider tfaddr.Provider, services *disco.Host) (Source, error) {
	// The following is a little hacky: we're instantiating the source that
	// was really written to handle the oci_mirror installation method, but
	// we're instantiating it with a repository address callback that just
	// always returns a fixed address and therefore can only answer questions
	// about the specific provider we've been given.
	// That's okay because the caller will only use the result to serve this
	// provider, but nonetheless it's still a bit of an abstraction inversion
	// to use the mirror source to implement the direct source.
	// If we decide to move forward with this (currently-experimental) capability
	// then hopefully we can refactor this to be a more sensible shape.
	hostname, name, err := services.ServiceOCIRepositoryFromURITemplateLevel1("oci-providers.v1", map[string]string{
		"namespace": provider.Namespace,
		"type":      provider.Type,
	})
	//nolint:errorlint // this is intentionally following the structure from RegistrySource, for now
	switch err := err.(type) {
	case nil:
		return NewOCIMirrorSource(func(_ tfaddr.Provider) (OCIRepository, tfdiags.Diagnostics) {
			return OCIRepository{
				Hostname: hostname,
				Name:     name,
			}, nil
		}), nil
	case *disco.ErrServiceNotProvided:
		return nil, ErrHostNoProviders{
			Hostname: provider.Hostname,
		}
	case *disco.ErrVersionNotSupported:
		return nil, ErrHostNoProviders{
			Hostname:        provider.Hostname,
			HasOtherVersion: true,
		}
	default:
		return nil, ErrHostUnreachable{
			Hostname: provider.Hostname,
			Wrapped:  err,
		}
	}
}
