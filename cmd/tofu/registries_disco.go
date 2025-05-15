// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/opentofu/svchost/disco"
	"github.com/opentofu/svchost/svcauth"

	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/logging"
)

const (
	// registryDiscoveryRetryEnvName is the name of the environment variable that
	// can be configured to customize number of retries for module and provider
	// discovery requests with the remote registry.
	registryDiscoveryRetryEnvName      = "TF_REGISTRY_DISCOVERY_RETRY"
	registryDiscoveryDefaultRetryCount = 1

	// registryClientTimeoutEnvName is the name of the environment variable that
	// can be configured to customize the timeout duration (seconds) for module
	// and provider discovery with a remote registry. For historical reasons
	// this also applies to all service discovery requests regardless of whether
	// they are registry-related.
	registryClientTimeoutEnvName        = "TF_REGISTRY_CLIENT_TIMEOUT"
	registryClientDefaultRequestTimeout = 10 * time.Second
)

// newServiceDiscovery returns a newly-created [disco.Disco] object that is
// configured appropriately for use elsewhere in OpenTofu.
//
// The credSrc argument represents the policy for how the service discovery
// object should obtain authentication credentials for service discovery
// requests. Passing a nil credSrc is acceptable and means that all discovery
// requests are to be made anonymously.
func newServiceDiscovery(ctx context.Context, credSrc svcauth.CredentialsSource) *disco.Disco {
	// For historical reasons, the registry request retry policy also applies
	// to all service discovery requests, which we implement by using transport
	// from a HTTP httpClient that is configured for registry httpClient use.
	registryHTTPClient := newRegistryHTTPClient(ctx)
	services := disco.New(
		disco.WithHTTPClient(registryHTTPClient.HTTPClient),
		disco.WithCredentials(credSrc),
	)
	return services
}

// newRegistryHTTPClient returns a new HTTP client configured to respect the
// automatic retry behavior expected for registry requests and service discovery
// requests.
func newRegistryHTTPClient(ctx context.Context) *retryablehttp.Client {
	// The retry count is configurable by environment variable.
	retryCount := registryDiscoveryDefaultRetryCount
	if v := os.Getenv(registryDiscoveryRetryEnvName); v != "" {
		override, err := strconv.Atoi(v)
		if err == nil && override > 0 {
			retryCount = override
		}
	}

	// The timeout is also configurable by environment variable.
	timeout := registryClientDefaultRequestTimeout
	if v := os.Getenv(registryClientTimeoutEnvName); v != "" {
		override, err := strconv.Atoi(v)
		if err == nil && timeout > 0 {
			timeout = time.Duration(override) * time.Second
		}
	}

	client := httpclient.NewForRegistryRequests(ctx, retryCount, timeout)

	// Per historical tradition our registry client also generates log messages
	// describing the requests that it makes.
	logOutput := logging.LogOutput()
	client.Logger = log.New(logOutput, "", log.Flags())

	return client
}
