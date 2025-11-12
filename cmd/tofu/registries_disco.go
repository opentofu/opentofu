// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"log"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/opentofu/svchost/disco"
	"github.com/opentofu/svchost/svcauth"

	"github.com/opentofu/opentofu/internal/command/cliconfig"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/logging"
)

// newServiceDiscovery returns a newly-created [disco.Disco] object that is
// configured appropriately for use elsewhere in OpenTofu.
//
// The credSrc argument represents the policy for how the service discovery
// object should obtain authentication credentials for service discovery
// requests. Passing a nil credSrc is acceptable and means that all discovery
// requests are to be made anonymously.
func newServiceDiscovery(ctx context.Context, registryClientConfig *cliconfig.RegistryProtocolsConfig, credSrc svcauth.CredentialsSource) *disco.Disco {
	// For historical reasons, the registry request retry policy also applies
	// to all service discovery requests, which we implement by using transport
	// from a HTTP httpClient that is configured for registry httpClient use.
	registryHTTPClient := newRegistryHTTPClient(ctx, registryClientConfig)
	services := disco.New(
		disco.WithHTTPClient(registryHTTPClient.HTTPClient),
		disco.WithCredentials(credSrc),
	)
	return services
}

// newRegistryHTTPClient returns a new HTTP client configured to respect the
// automatic retry behavior expected for registry requests and service discovery
// requests.
func newRegistryHTTPClient(ctx context.Context, config *cliconfig.RegistryProtocolsConfig) *retryablehttp.Client {
	client := httpclient.NewForRegistryRequests(ctx, config.RetryCount, config.RequestTimeout)

	// Per historical tradition our registry client also generates log messages
	// describing the requests that it makes.
	logOutput := logging.LogOutput()
	client.Logger = log.New(logOutput, "", log.Flags())

	return client
}
