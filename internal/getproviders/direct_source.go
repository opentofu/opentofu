// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/apparentlymart/go-versions/versions"
	svchost "github.com/hashicorp/terraform-svchost"
	disco "github.com/hashicorp/terraform-svchost/disco"
	"github.com/opentofu/libregistry/registryprotocols/ociclient"
	tfaddr "github.com/opentofu/registry-address"
)

// DirectSource is a Source that handles the "direct" installation method by
// performing service discovery on a provider's origin registry hostname and
// then using the declared services to decide how to handle the request.
type DirectSource struct {
	services      *disco.Disco
	ociDistClient ociclient.OCIClient
}

var _ Source = (*DirectSource)(nil)

// NewRegistrySource creates and returns a new source that will install
// providers from their originating provider registries.
func NewDirectSource(services *disco.Disco, ociDistClient ociclient.OCIClient) *DirectSource {
	return &DirectSource{
		services:      services,
		ociDistClient: ociDistClient,
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
	if isMagicOCIMirrorHost(provider.Hostname) {
		if s.ociDistClient == nil {
			// If the OCI distribution client is not available then we can't support this case.
			// (This should happen only if the CLI configuration's OCI registry configuration
			// is so invalid that we couldn't even instantiate the client, and package main
			// should already have complained about that earlier.)
			return nil, fmt.Errorf("no OCI distribution client is available")
		}
		log.Printf("[TRACE] DirectSource: using magic OCI distribution registry mapping for %s", provider.Hostname.ForDisplay())
		return s.magicOCIMirrorSource(ctx, provider)
	}

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
	hostname, name, err := services.ServiceOCIRepositoryFromURITemplateLevel1("oci-providers.v1", map[string]string{
		"namespace": provider.Namespace,
		"type":      provider.Type,
	})
	//nolint:errorlint // this is intentionally following the structure from RegistrySource, for now
	switch err := err.(type) {
	case nil:
		if s.ociDistClient == nil {
			// If the OCI distribution client is not available then we can't support this case.
			// (This should happen only if the CLI configuration's OCI registry configuration
			// is so invalid that we couldn't even instantiate the client, and package main
			// should already have complained about that earlier.)
			return nil, fmt.Errorf("no OCI distribution client is available")
		}

		// As an implementation detail we use a specially-configured OCIMirrorSource that
		// always installs from the specific OCI repository address we've just discovered.
		return newOCIMirrorSourceForDirectInstall(s.ociDistClient, OCIRepository{
			Hostname: hostname,
			Name:     name,
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

// magicOCIRegistryHostnameSuffix is a special hostname suffix that causes OpenTofu
// to skip network service discovery and instead just behave as if "oci-providers.v1"
// were implemented for the prefix that remains after removing this suffix.
//
// FIXME: This is currently just a placeholder for experimentation. If we decide to
// actually follow this strategy then we should replace this with a domain we
// actually own.
const magicOCIRegistryHostnameSuffix = ".opentofu-oci.example.com"

func isMagicOCIMirrorHost(hostname svchost.Hostname) bool {
	return strings.HasSuffix(hostname.String(), magicOCIRegistryHostnameSuffix)
}

// magicOCIMirrorSource handles the special case where hostnames ending
// in [magicOCIRegistryHostnameSuffix] are forced to behave as if they
// declared the "oci-providers.v1" service with a fixed template.
func (s *DirectSource) magicOCIMirrorSource(_ context.Context, provider tfaddr.Provider) (Source, error) {
	fullHostname := provider.Hostname.String()
	ociRegistryHost := fullHostname[:len(fullHostname)-len(magicOCIRegistryHostnameSuffix)]
	ociRepositoryName := provider.Namespace + "/opentofu-provider-" + provider.Type

	// As an implementation detail we use a specially-configured OCIMirrorSource that
	// always installs from the specific OCI repository address we've just inferred
	// "magically" based on the provider address.
	return newOCIMirrorSourceForDirectInstall(s.ociDistClient, OCIRepository{
		Hostname: ociRegistryHost,
		Name:     ociRepositoryName,
	}), nil
}
