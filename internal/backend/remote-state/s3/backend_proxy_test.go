// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package s3

import (
	"testing"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/testutils"
)

func TestS3ProxyBehavior(t *testing.T) {
	testutils.SetupTestLogger(t)

	aws := testutils.AWS(t)

	t.Run("direct-connect", func(t *testing.T) {
		testutils.SetupTestLogger(t)
		config := map[string]any{
			"access_key":       aws.AccessKey(),
			"secret_key":       aws.SecretKey(),
			"region":           aws.Region(),
			"bucket":           aws.S3Bucket(),
			"key":              t.Name(),
			"encrypt":          true,
			"custom_ca_bundle": aws.CACertFile(),
			"use_path_style":   aws.S3UsePathStyle(),
			"endpoints": map[string]any{
				"s3":       aws.S3Endpoint(),
				"iam":      aws.IAMEndpoint(),
				"sts":      aws.STSEndpoint(),
				"dynamodb": aws.DynamoDBEndpoint(),
			},
		}
		runBackendTests(t, config)
	})
	t.Run("proxy", func(t *testing.T) {
		var proxyTestCases = map[string]struct {
			modifyConfig func(t *testing.T, config map[string]any, proxy testutils.HTTPProxyService)
		}{
			// Intentionally using the HTTP proxy URL in these cases even for HTTPS to make sure we don't have to deal
			// with certificate issues. In this case the "https" refers to the target (endpoint) URL, not the proxy
			// itself.
			"direct-config": {
				modifyConfig: func(_ *testing.T, config map[string]any, proxy testutils.HTTPProxyService) {
					config["http_proxy"] = proxy.HTTPProxy().String()
					config["https_proxy"] = proxy.HTTPProxy().String()
				},
			},
			"env-uppercase": {
				modifyConfig: func(t *testing.T, config map[string]any, proxy testutils.HTTPProxyService) {
					t.Setenv("HTTP_PROXY", proxy.HTTPProxy().String())
					t.Setenv("HTTPS_PROXY", proxy.HTTPProxy().String())
				},
			},
			"env-lowercase": {
				modifyConfig: func(t *testing.T, config map[string]any, proxy testutils.HTTPProxyService) {
					t.Setenv("http_proxy", proxy.HTTPProxy().String())
					t.Setenv("https_proxy", proxy.HTTPProxy().String())
				},
			},
		}
		for name, tc := range proxyTestCases {
			t.Run(name, func(t *testing.T) {
				testutils.SetupTestLogger(t)
				// Override the target the proxy connects to in order to redirect requests to the correct backend.
				proxy := testutils.HTTPProxy(
					t,
					testutils.HTTPProxyOptionForceCONNECTTarget(aws.S3Endpoint()),
					testutils.HTTPProxyOptionForceHTTPTarget(aws.S3Endpoint()),
					testutils.HTTPProxyOptionForceHTTPSTarget(aws.S3Endpoint(), aws.CACert()),
				)
				config := map[string]any{
					"access_key":       aws.AccessKey(),
					"secret_key":       aws.SecretKey(),
					"region":           aws.Region(),
					"bucket":           aws.S3Bucket(),
					"key":              t.Name(),
					"encrypt":          false,
					"custom_ca_bundle": aws.CACertFile(),
					"use_path_style":   aws.S3UsePathStyle(),
					// Disable validations because the forced proxy redirect may not work if the endpoint URLs are different.
					// The proxy is not smart enough to distinguish between multiple endpoints.
					"skip_credentials_validation": true,
					"skip_region_validation":      true,
					"skip_metadata_api_check":     true,
					"skip_requesting_account_id":  true,
					// Override the endpoints to make sure they can't possibly connect unless we are using the proxy.
					// Note that this CANNOT be a loopback address, otherwise the Go proxy behavior decides that it should
					// bypass the proxy. Instead, we are using the RFC3849 IPv6 reserved range for documentation as this range
					// should never be routed to the public internet, so even if the test fails, it shouldn't cause any
					// damage.
					"endpoints": map[string]any{
						"s3":       "http://[2001:0DB8::1]:1",
						"iam":      "http://[2001:0DB8::1]:1",
						"sts":      "http://[2001:0DB8::1]:1",
						"dynamodb": "http://[2001:0DB8::1]:1",
					},
				}
				tc.modifyConfig(t, config, proxy)
				runBackendTests(t, config)
			})
		}
	})
}

func runBackendTests(t *testing.T, config map[string]any) {
	b := backend.TestTypedBackendConfig[*Backend](t, NewTyped(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(config))

	state, err := b.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	remote.TestClient(t, state.(*remote.State).Client)
}
