// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/opentofu/svchost"
	"github.com/opentofu/svchost/svcauth"
)

func TestPackageLocationHTTPArchive(t *testing.T) {
	authToken := "my-secret-token"
	wrongToken := "wrong-token"

	// 1. Create a mock HTTP server that expects an Authorization header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+authToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Respond with a valid minimal ZIP archive structure to allow the extractor stage to succeed
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PK\x05\x06\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"))
	}))
	defer ts.Close()

	// 2. Parse the test server URL to know the exact Hostname the client will target
	parsedURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}
	targetHost := svchost.Hostname(parsedURL.Host)

	// 3. Setup a temporary directory for extraction
	tmpDir, err := os.MkdirTemp("", "opentofu-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 4. Define the table-driven test cases using svcauth.StaticCredentialsSource
	// Change *svcauth.CredentialsSource to svcauth.CredentialsSource
	tests := map[string]struct {
		credsSource svcauth.CredentialsSource
		expectError bool
	}{
		"happy case": {
			credsSource: svcauth.StaticCredentialsSource(map[svchost.Hostname]svcauth.HostCredentials{
				targetHost: svcauth.HostCredentialsToken(authToken),
			}),
			expectError: false,
		},
		"unhappy: wrong hostname": {
			credsSource: svcauth.StaticCredentialsSource(map[svchost.Hostname]svcauth.HostCredentials{
				"other.host": svcauth.HostCredentialsToken(authToken),
			}),
			expectError: true,
		},
		"unhappy: wrong token": {
			credsSource: svcauth.StaticCredentialsSource(map[svchost.Hostname]svcauth.HostCredentials{
				targetHost: svcauth.HostCredentialsToken(wrongToken),
			}),
			expectError: true,
		},
		"unhappy: empty credential store": {
			credsSource: svcauth.StaticCredentialsSource(map[svchost.Hostname]svcauth.HostCredentials{}),
			expectError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			loc := PackageHTTPURL{
				URL: ts.URL,
				ClientBuilder: func(ctx context.Context) *retryablehttp.Client {
					client := retryablehttp.NewClient()
					client.Logger = nil // Keep testing output clean
					return client
				},
			}

			meta := PackageMeta{
				Location:          loc,
				CredentialsSource: tc.credsSource,
			}

			_, err := loc.InstallProviderPackage(ctx, meta, tmpDir, nil)

			if tc.expectError {
				if err == nil {
					t.Errorf("expected download to fail, but it succeeded")
				}
			} else {
				if err != nil {
					t.Errorf("expected download to succeed, but got error: %v", err)
				}
			}
		})
	}
}
