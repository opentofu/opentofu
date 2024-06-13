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
		// Override the target the proxy connects to in order to redirect requests to the correct backend.
		proxy := testutils.HTTPProxy(
			t,
			testutils.HTTPProxyOptionForceCONNECTTarget(aws.S3Endpoint()),
			testutils.HTTPProxyOptionForceHTTPTarget(aws.S3Endpoint()),
			testutils.HTTPProxyOptionForceHTTPSTarget(aws.S3Endpoint(), aws.CACert()),
		)
		config := map[string]any{
			"access_key": aws.AccessKey(),
			"secret_key": aws.SecretKey(),
			"region":     aws.Region(),
			"bucket":     aws.S3Bucket(),
			"http_proxy": proxy.HTTPProxy().String(),
			// Intentionally using the HTTP proxy URL to make sure we don't have to deal with certificate
			// issues. In this case the "https" refers to the target (endpoint) URL, not the proxy itself.
			"https_proxy":      proxy.HTTPProxy().String(),
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
			// Override the endpoints to make sure they can't possibly connect unless we are using the proxy:
			"endpoints": map[string]any{
				"s3":       "http://127.0.0.1:1",
				"iam":      "http://127.0.0.1:1",
				"sts":      "http://127.0.0.1:1",
				"dynamodb": "http://127.0.0.1:1",
			},
		}
		runBackendTests(t, config)
	})
}

func runBackendTests(t *testing.T, config map[string]any) {
	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(config)).(*Backend)

	state, err := b.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	remote.TestClient(t, state.(*remote.State).Client)
}
