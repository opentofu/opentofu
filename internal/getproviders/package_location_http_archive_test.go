// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
)

// mockCredentialsSource implements the credentials source interface for testing.
type mockCredentialsSource struct {
	token string
}

func (m *mockCredentialsSource) ForHost(ctx context.Context, host string) (*mockCredentials, error) {
	if m.token == "" {
		return nil, nil
	}
	return &mockCredentials{token: m.token}, nil
}

// mockCredentials satisfies the expected credentials interface with a PrepareRequest method.
type mockCredentials struct {
	token string
}

func (c *mockCredentials) PrepareRequest(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token)
}

func TestPackageLocationHTTPArchive(t *testing.T) {
	// Create a test server that expects an Authorization header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer valid-token-123" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Respond with a valid minimal ZIP structure to appease the extraction stage
		// (PK\x05\x06 followed by 18 bytes of zeros is an empty zip archive)
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PK\x05\x06\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"))
	}))
	defer ts.Close()

	// Setup a temporary target directory for the extraction process
	tmpDir, err := os.MkdirTemp("", "opentofu-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := map[string]struct {
		credsSource *mockCredentialsSource
		expectError bool
	}{
		"success_with_valid_token": {
			credsSource: &mockCredentialsSource{token: "valid-token-123"},
			expectError: false,
		},
		"failure_without_token": {
			credsSource: &mockCredentialsSource{token: ""}, // No token set
			expectError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			// Build the Location struct injecting a default retryable client
			loc := PackageHTTPURL{
				URL: ts.URL,
				ClientBuilder: func(ctx context.Context) *retryablehttp.Client {
					client := retryablehttp.NewClient()
					client.Logger = nil // Suppress noise in test output
					return client
				},
			}

			meta := PackageMeta{
				Location:          loc,
				CredentialsSource: tc.credsSource,
			}

			// We pass nil for allowedHashes here as an empty zip needs no complex verification
			_, err := loc.InstallProviderPackage(ctx, meta, tmpDir, nil)

			if tc.expectError {
				if err == nil {
					t.Errorf("expected an error but installation succeeded")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
