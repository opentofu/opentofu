// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package r2

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

func TestBackend_impl(t *testing.T) {
	var _ backend.Backend = (*Backend)(nil)
}

func TestBackendConfig(t *testing.T) {
	b := New(encryption.StateEncryptionDisabled())
	schema := b.ConfigSchema()

	// Test required attributes
	requiredAttrs := []string{"account_id", "api_token", "bucket", "key"}
	for _, attr := range requiredAttrs {
		if v, ok := schema.Attributes[attr]; !ok {
			t.Errorf("missing required attribute %q", attr)
		} else if !v.Required {
			t.Errorf("attribute %q should be required", attr)
		}
	}

	// Test optional attributes
	optionalAttrs := []string{"workspace_key_prefix", "endpoint", "jurisdiction"}
	for _, attr := range optionalAttrs {
		if v, ok := schema.Attributes[attr]; !ok {
			t.Errorf("missing optional attribute %q", attr)
		} else if v.Required {
			t.Errorf("attribute %q should be optional", attr)
		}
	}

	// Test sensitive attributes
	if !schema.Attributes["api_token"].Sensitive {
		t.Error("api_token should be marked as sensitive")
	}
}

func TestBackendConfig_PrepareConfig(t *testing.T) {
	b := New(encryption.StateEncryptionDisabled())

	tests := []struct {
		name      string
		config    map[string]cty.Value
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid config",
			config: map[string]cty.Value{
				"account_id": cty.StringVal("1234567890abcdef1234567890abcdef"),
				"api_token":  cty.StringVal("test-token"),
				"bucket":     cty.StringVal("test-bucket"),
				"key":        cty.StringVal("test.tfstate"),
			},
			wantError: false,
		},
		{
			name: "invalid account_id",
			config: map[string]cty.Value{
				"account_id": cty.StringVal("invalid"),
				"api_token":  cty.StringVal("test-token"),
				"bucket":     cty.StringVal("test-bucket"),
				"key":        cty.StringVal("test.tfstate"),
			},
			wantError: true,
			errorMsg:  "32-character hexadecimal",
		},
		{
			name: "invalid bucket name - too short",
			config: map[string]cty.Value{
				"account_id": cty.StringVal("1234567890abcdef1234567890abcdef"),
				"api_token":  cty.StringVal("test-token"),
				"bucket":     cty.StringVal("ab"),
				"key":        cty.StringVal("test.tfstate"),
			},
			wantError: true,
			errorMsg:  "between 3 and 63 characters",
		},
		{
			name: "invalid bucket name - uppercase",
			config: map[string]cty.Value{
				"account_id": cty.StringVal("1234567890abcdef1234567890abcdef"),
				"api_token":  cty.StringVal("test-token"),
				"bucket":     cty.StringVal("Test-Bucket"),
				"key":        cty.StringVal("test.tfstate"),
			},
			wantError: true,
			errorMsg:  "lowercase letters",
		},
		{
			name: "invalid key - starts with slash",
			config: map[string]cty.Value{
				"account_id": cty.StringVal("1234567890abcdef1234567890abcdef"),
				"api_token":  cty.StringVal("test-token"),
				"bucket":     cty.StringVal("test-bucket"),
				"key":        cty.StringVal("/test.tfstate"),
			},
			wantError: true,
			errorMsg:  "cannot start with '/'",
		},
		{
			name: "invalid jurisdiction",
			config: map[string]cty.Value{
				"account_id":   cty.StringVal("1234567890abcdef1234567890abcdef"),
				"api_token":    cty.StringVal("test-token"),
				"bucket":       cty.StringVal("test-bucket"),
				"key":          cty.StringVal("test.tfstate"),
				"jurisdiction": cty.StringVal("invalid"),
			},
			wantError: true,
			errorMsg:  "must be either 'eu' or 'fedramp'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Add defaults for optional fields
			config := make(map[string]cty.Value)
			for k, v := range tt.config {
				config[k] = v
			}
			
			// Add nulls for optional fields if not present
			if _, ok := config["workspace_key_prefix"]; !ok {
				config["workspace_key_prefix"] = cty.NullVal(cty.String)
			}
			if _, ok := config["endpoint"]; !ok {
				config["endpoint"] = cty.NullVal(cty.String)
			}
			if _, ok := config["jurisdiction"]; !ok {
				config["jurisdiction"] = cty.NullVal(cty.String)
			}
			
			configVal := cty.ObjectVal(config)
			
			_, diags := b.PrepareConfig(configVal)
			
			if tt.wantError {
				if !diags.HasErrors() {
					t.Fatal("expected error but got none")
				}
				if tt.errorMsg != "" {
					found := false
					for _, diag := range diags {
						if diag.Severity() == tfdiags.Error {
							detail := diag.Description().Detail
							if detail != "" && contains(detail, tt.errorMsg) {
								found = true
								break
							}
						}
					}
					if !found {
						t.Errorf("expected error containing %q, got %v", tt.errorMsg, diags.Err())
					}
				}
			} else {
				if diags.HasErrors() {
					t.Fatalf("unexpected error: %v", diags.Err())
				}
			}
		})
	}
}

func TestBackend_Configure(t *testing.T) {
	// Create a test server to mock Cloudflare API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/accounts/1234567890abcdef1234567890abcdef/r2/buckets/test-bucket":
			if r.Method == "GET" {
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, `{
					"success": true,
					"result": {
						"name": "test-bucket",
						"creation_date": "2023-01-01T00:00:00Z",
						"location": "auto"
					}
				}`)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	b := New(encryption.StateEncryptionDisabled())
	
	config := cty.ObjectVal(map[string]cty.Value{
		"account_id":          cty.StringVal("1234567890abcdef1234567890abcdef"),
		"api_token":           cty.StringVal("test-token"),
		"bucket":              cty.StringVal("test-bucket"),
		"key":                 cty.StringVal("test.tfstate"),
		"workspace_key_prefix": cty.StringVal("env:"),
		"endpoint":            cty.StringVal(server.URL),
		"jurisdiction":        cty.StringVal(""),
	})

	diags := b.Configure(context.Background(), config)
	if diags.HasErrors() {
		t.Fatalf("unexpected error: %v", diags.Err())
	}

	// Verify configuration was set correctly
	backend := b.(*Backend)
	if backend.accountID != "1234567890abcdef1234567890abcdef" {
		t.Errorf("expected account_id to be set")
	}
	if backend.bucketName != "test-bucket" {
		t.Errorf("expected bucket to be set")
	}
	if backend.key != "test.tfstate" {
		t.Errorf("expected key to be set")
	}
	if backend.workspaceKeyPrefix != "env:" {
		t.Errorf("expected workspace_key_prefix to be 'env:', got %q", backend.workspaceKeyPrefix)
	}
}

func TestBackend_verifyBucketAccess(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantError  bool
		errorMsg   string
	}{
		{
			name:       "bucket exists",
			statusCode: http.StatusOK,
			wantError:  false,
		},
		{
			name:       "bucket not found",
			statusCode: http.StatusNotFound,
			wantError:  true,
			errorMsg:   "bucket not found",
		},
		{
			name:       "unauthorized",
			statusCode: http.StatusUnauthorized,
			wantError:  true,
			errorMsg:   "unexpected status code: 401",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify authentication header
				if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
					t.Errorf("expected Authorization header, got %q", auth)
				}
				
				w.WriteHeader(tt.statusCode)
				if tt.statusCode == http.StatusOK {
					fmt.Fprintf(w, `{"success": true, "result": {"name": "test-bucket"}}`)
				}
			}))
			defer server.Close()

			b := &Backend{
				accountID:  "1234567890abcdef1234567890abcdef",
				apiToken:   "test-token",
				bucketName: "test-bucket",
				endpoint:   server.URL,
				httpClient: httpclient.New(context.Background()),
			}

			err := b.verifyBucketAccess(context.Background())
			
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestBackend_getR2Endpoint(t *testing.T) {
	tests := []struct {
		name         string
		jurisdiction string
		endpoint     string
		accountID    string
		want         string
	}{
		{
			name:         "default jurisdiction",
			jurisdiction: "",
			accountID:    "abc123",
			want:         "https://abc123.r2.cloudflarestorage.com",
		},
		{
			name:         "EU jurisdiction",
			jurisdiction: "eu",
			accountID:    "abc123",
			want:         "https://abc123.eu.r2.cloudflarestorage.com",
		},
		{
			name:         "FedRAMP jurisdiction",
			jurisdiction: "fedramp",
			accountID:    "abc123",
			want:         "https://abc123.fedramp.r2.cloudflarestorage.com",
		},
		{
			name:         "custom endpoint",
			jurisdiction: "eu",
			endpoint:     "https://custom.endpoint.com",
			accountID:    "abc123",
			want:         "https://custom.endpoint.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Backend{
				accountID:    tt.accountID,
				jurisdiction: tt.jurisdiction,
				endpoint:     tt.endpoint,
			}

			got := b.getR2Endpoint()
			if got != tt.want {
				t.Errorf("getR2Endpoint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || 
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
