// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

// This file deals with our cross-cutting concerns relating to the OCI Distribution
// protocol, shared across both the provider and module installers, and potentially
// other OCI Registry concerns in future.

import (
	"context"
	"fmt"

	orasRemote "oras.land/oras-go/v2/registry/remote"
	orasAuth "oras.land/oras-go/v2/registry/remote/auth"

	"github.com/opentofu/opentofu/internal/command/cliconfig/ociauthconfig"
	"github.com/opentofu/opentofu/internal/getmodules"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/httpclient"
)

// ociCredsPolicyBuilder is the type of a callback function that the [providerSource]
// and [remoteModulePackageFetcher] functions will use if any of the configured
// provider installation methods or the module installer need to interact with
// OCI Distribution registries.
//
// We represent this indirectly as a callback function so that we can skip doing
// this work in the common case where we won't need to interact with OCI registries
// at all.
type ociCredsPolicyBuilder func(context.Context) (ociauthconfig.CredentialsConfigs, error)

// getOCIRepositoryStore instantiates a [getproviders.OCIRepositoryStore] implementation to use
// when accessing the given repository on the given registry, using the given OCI credentials
// policy to decide which credentials to use.
func getOCIRepositoryStore(ctx context.Context, registryDomain, repositoryName string, credsPolicy ociauthconfig.CredentialsConfigs) (ociRepositoryStore, error) {
	// We currently use the ORAS-Go library to satisfy both the [getproviders.OCIRepositoryStore]
	// and [getmodules.OCIRepositoryStore] interfaces, which is easy because those interfaces
	// were designed to match a subset of the ORAS-Go API since we had no particular need to
	// diverge from it. However, we consider ORAS-Go to be an implementation detail here and so
	// we should avoid any ORAS-Go types becoming part of the direct public API between packages.

	// ORAS-Go has a bit of an impedence mismatch with us in that it thinks of credentials
	// as being a per-registry thing rather than a per-repository thing, so we deal with
	// the credSource resolution ourselves here and then just return whatever we found to
	// ORAS when it asks through its callback. In practice we only interact with one
	// repository per client so this is just a little inconvenient and not a practical problem.
	credSource, err := credsPolicy.CredentialsSourceForRepository(ctx, registryDomain, repositoryName)
	if ociauthconfig.IsCredentialsNotFoundError(err) {
		credSource = nil // we'll just try without any credentials, then
	} else if err != nil {
		return nil, fmt.Errorf("finding credentials for %q: %w", registryDomain, err)
	}
	client := &orasAuth.Client{
		Client: httpclient.New(), // the underlying HTTP client to use, preconfigured with OpenTofu's User-Agent string
		Credential: func(ctx context.Context, hostport string) (orasAuth.Credential, error) {
			if hostport != registryDomain {
				// We should not send the credentials we selected to any registry
				// other than the one they were configured for.
				return orasAuth.EmptyCredential, nil
			}
			if credSource == nil {
				return orasAuth.EmptyCredential, nil
			}
			creds, err := credSource.Credentials(ctx, ociCredentialsLookupEnv{})
			if ociauthconfig.IsCredentialsNotFoundError(err) {
				return orasAuth.EmptyCredential, nil
			}
			if err != nil {
				return orasAuth.Credential{}, err
			}
			return creds.ToORASCredential(), nil
		},
		Cache: orasAuth.NewCache(),
	}
	reg, err := orasRemote.NewRegistry(registryDomain)
	if err != nil {
		return nil, err // This is only for registryDomain validation errors, and we should've caught those much earlier than here
	}
	reg.Client = client
	err = reg.Ping(ctx) // tests whether the given domain refers to a valid OCI repository and will accept the credentials
	if err != nil {
		return nil, fmt.Errorf("failed to contact OCI registry at %q: %w", registryDomain, err)
	}
	repo, err := reg.Repository(ctx, repositoryName)
	if err != nil {
		return nil, err // This is only for repositoryName validation errors, and we should've caught those much earlier than here
	}
	// NOTE: At this point we don't yet know if the named repository actually exists
	// in the registry. The caller will find that out when they try to interact
	// with the methods of the returned object.
	return repo, nil
}

// ociRepositoryStore represents the combined needs of both
// [getproviders.OCIRepositoryStore] and [getmodules.OCIRepositoryStore],
// both of which are intentionally defined to be subsets of the API
// used by ORAS-Go so that we can use the implementations from that
// library without directly exposing any ORAS-Go symbols in the
// public API of any of our packages, since we want to reserve the
// ability to switch to other implementations in future if needed.
type ociRepositoryStore interface {
	getproviders.OCIRepositoryStore
	getmodules.OCIRepositoryStore
}

// ociCredentialsLookupEnv is our implementation of ociauthconfig.CredentialsLookupEnvironment
// used when resolving the selected credentials for a particular OCI repository.
type ociCredentialsLookupEnv struct{}

var _ ociauthconfig.CredentialsLookupEnvironment = ociCredentialsLookupEnv{}

// QueryDockerCredentialHelper implements ociauthconfig.CredentialsLookupEnvironment.
func (o ociCredentialsLookupEnv) QueryDockerCredentialHelper(ctx context.Context, helperName string, serverURL string) (ociauthconfig.DockerCredentialHelperGetResult, error) {
	// TODO: Implement this
	return ociauthconfig.DockerCredentialHelperGetResult{}, fmt.Errorf("support for Docker-style credential helpers is not yet available")
}
