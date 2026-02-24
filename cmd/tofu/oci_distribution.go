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
	"log"
	"sync"

	orasRemote "oras.land/oras-go/v2/registry/remote"
	orasAuth "oras.land/oras-go/v2/registry/remote/auth"
	orasCreds "oras.land/oras-go/v2/registry/remote/credentials"
	orasCredsTrace "oras.land/oras-go/v2/registry/remote/credentials/trace"

	"github.com/opentofu/opentofu/internal/command/cliconfig/ociauthconfig"
	"github.com/opentofu/opentofu/internal/getmodules"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/tracing"
	"github.com/opentofu/opentofu/internal/tracing/traceattrs"
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

var ociReposMu sync.Mutex
var ociRepos map[ociRepoKey]ociRepositoryStore

type ociRepoKey struct {
	registryDomain, repositoryName string
}

// getOCIRepositoryStore instantiates a [getproviders.OCIRepositoryStore] implementation to use
// when accessing the given repository on the given registry, using the given OCI credentials
// policy to decide which credentials to use.
//
// This function attempts to reuse previously-instantiated stores for a given registry
// domain and repository name, and so it effectively assumes that all calls through the
// life of the program will have the same credsPolicy argument. That assumption should
// hold because in practice we only create a single credsPolicy per execution, based on
// the CLI Configuration, and use it in both module_source.go and provider_source.go.
func getOCIRepositoryStore(ctx context.Context, registryDomain, repositoryName string, credsPolicy ociauthconfig.CredentialsConfigs) (ociRepositoryStore, error) {
	// We currently use the ORAS-Go library to satisfy both the [getproviders.OCIRepositoryStore]
	// and [getmodules.OCIRepositoryStore] interfaces, which is easy because those interfaces
	// were designed to match a subset of the ORAS-Go API since we had no particular need to
	// diverge from it. However, we consider ORAS-Go to be an implementation detail here and so
	// we should avoid any ORAS-Go types becoming part of the direct public API between packages.

	ociReposMu.Lock()
	defer ociReposMu.Unlock()
	if ociRepos == nil {
		ociRepos = make(map[ociRepoKey]ociRepositoryStore)
	}
	// Reused cached store if possible, since that potentially allows us to
	// reuse a previously-issued temporary auth token and thus skip a few
	// session-setup roundtrips to the registry API.
	key := ociRepoKey{registryDomain, repositoryName}
	if store, ok := ociRepos[key]; ok {
		return store, nil
	}

	ctx, span := tracing.Tracer().Start(
		ctx, "Authenticate to OCI Registry",
		tracing.SpanAttributes(
			traceattrs.OpenTofuOCIRegistryDomain(registryDomain),
			traceattrs.OpenTofuOCIRepositoryName(repositoryName),
		),
	)
	defer span.End()

	// Since there are lots of different ways to provide OCI credentials to
	// OpenTofu, and several are implicit based on files and/or environment
	// variables we found on the system, we'll generate some debug logs
	// listing the locations where we're searching so we'll have some good
	// context for a bug report about OpenTofu selecting different credentials
	// than the operator expected. There should not typically be more than a
	// few of these on a reasonably-configured system.
	for _, cfg := range credsPolicy.AllConfigs() {
		log.Printf("[DEBUG] OCI registry client will consider credentials from %s", cfg.CredentialsConfigLocationForUI())
	}

	client, err := getOCIRepositoryORASClient(ctx, registryDomain, repositoryName, credsPolicy)
	if err != nil {
		tracing.SetSpanError(span, err)
		return nil, err
	}
	reg, err := orasRemote.NewRegistry(registryDomain)
	if err != nil {
		tracing.SetSpanError(span, err)
		return nil, err // This is only for registryDomain validation errors, and we should've caught those much earlier than here
	}
	reg.Client = client
	err = reg.Ping(ctx) // tests whether the given domain refers to a valid OCI repository and will accept the credentials
	if err != nil {
		tracing.SetSpanError(span, err)
		return nil, fmt.Errorf("failed to contact OCI registry at %q: %w", registryDomain, err)
	}
	repo, err := reg.Repository(ctx, repositoryName)
	if err != nil {
		tracing.SetSpanError(span, err)
		return nil, err // This is only for repositoryName validation errors, and we should've caught those much earlier than here
	}

	// Save this in case we get asked again for the same registry.
	// (A subsequent call is common for provider installation since there
	// are several independent steps that all request stores separately.)
	ociRepos[key] = repo

	// NOTE: At this point we don't yet know if the named repository actually exists
	// in the registry. The caller will find that out when they try to interact
	// with the methods of the returned object.
	return repo, nil
}

func getOCIRepositoryORASClient(ctx context.Context, registryDomain, repositoryName string, credsPolicy ociauthconfig.CredentialsConfigs) (*orasAuth.Client, error) {
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
	return &orasAuth.Client{
		Client: httpclient.New(ctx), // the underlying HTTP client to use, preconfigured with OpenTofu's User-Agent string
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
	}, nil
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
	// (just because this type name is very long to keep repeating in full)
	type Result = ociauthconfig.DockerCredentialHelperGetResult

	ctx, span := tracing.Tracer().Start(
		ctx, "Query Docker-style credential helper",
		tracing.SpanAttributes(
			traceattrs.String("opentofu.oci.docker_credential_helper.name", helperName),
			traceattrs.String("opentofu.oci.registry.url", serverURL),
		),
	)
	defer span.End()

	// We currently use the ORAS-Go implementation of the Docker
	// credential helper protocol, because we already depend on
	// that library for our OCI registry interactions elsewhere.
	// ORAS refers to this protocol as "native store", rather
	// than "Docker-style Credential Helper", but it's the
	// same protocol nonetheless.

	var executeSpan tracing.Span // ORAS tracing API can't directly propagate span from Start to Done
	ctx = orasCredsTrace.WithExecutableTrace(ctx, &orasCredsTrace.ExecutableTrace{
		ExecuteStart: func(executableName, action string) {
			_, executeSpan = tracing.Tracer().Start(
				ctx, "Execute helper program",
				tracing.SpanAttributes(
					traceattrs.String("opentofu.oci.docker_credential_helper.executable", helperName),
					traceattrs.String("opentofu.oci.registry.url", serverURL),
				),
			)
			log.Printf("[DEBUG] Executing docker-style credentials helper %q for %s", helperName, serverURL)
		},
		ExecuteDone: func(executableName, action string, err error) {
			if executeSpan != nil {
				tracing.SetSpanError(executeSpan, err)
				executeSpan.End()
			}
			if err != nil {
				log.Printf("[ERROR] Docker-style credential helper %q failed for %s: %s", helperName, serverURL, err)
			}
		},
	})

	store := orasCreds.NewNativeStore(helperName)
	creds, err := store.Get(ctx, serverURL)
	if err != nil {
		tracing.SetSpanError(span, err)
		return Result{}, fmt.Errorf("%q credential helper failed: %w", helperName, err)
	}
	if creds.AccessToken != "" || creds.RefreshToken != "" {
		// A little awkward: orasAuth.Credential is a more general type than
		// what the Docker credential helper needs: it has fields for OAuth-style
		// credentials even though the credential helper protocol only supports
		// username/password style. So for completeness/robustness we check
		// the OAuth fields and fail if they are set, but it should not actually
		// be possible for them to be set in practice.
		err := fmt.Errorf("%q credential helper returned OAuth-style credentials, but only username/password-style is allowed from a credential helper", helperName)
		tracing.SetSpanError(span, err)
		return Result{}, err
	}
	return Result{
		ServerURL: serverURL,
		Username:  creds.Username,
		Secret:    creds.Password,
	}, nil
}
