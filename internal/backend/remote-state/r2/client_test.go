// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package r2

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/states/remote"
)

func TestRemoteClient_impl(t *testing.T) {
	var _ remote.Client = (*RemoteClient)(nil)
}

func TestRemoteClient_Get(t *testing.T) {
	testCases := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectNil      bool
		expectError    bool
		expectedErrMsg string
	}{
		{
			name:         "successful get",
			statusCode:   http.StatusOK,
			responseBody: `{"version": 4, "terraform_version": "1.0.0"}`,
			expectNil:    false,
			expectError:  false,
		},
		{
			name:        "not found",
			statusCode:  http.StatusNotFound,
			expectNil:   true,
			expectError: false,
		},
		{
			name:           "server error",
			statusCode:     http.StatusInternalServerError,
			responseBody:   "Internal Server Error",
			expectNil:      true,
			expectError:    true,
			expectedErrMsg: "failed to get state",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("expected GET request, got %s", r.Method)
				}
				
				// Verify authorization header
				if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
					t.Errorf("expected Authorization header 'Bearer test-token', got %q", auth)
				}
				
				w.WriteHeader(tc.statusCode)
				if tc.responseBody != "" {
					_, _ = w.Write([]byte(tc.responseBody))
				}
			}))
			defer server.Close()

			backend := &Backend{
				apiToken:   "test-token",
				httpClient: httpclient.New(context.Background()),
				endpoint:   server.URL,
			}

			client := &RemoteClient{
				backend:    backend,
				bucketName: "test-bucket",
				key:        "test.tfstate",
			}

			payload, err := client.Get(context.Background())

			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if tc.expectedErrMsg != "" && !contains(err.Error(), tc.expectedErrMsg) {
					t.Errorf("expected error containing %q, got %v", tc.expectedErrMsg, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if tc.expectNil {
				if payload != nil {
					t.Error("expected nil payload")
				}
			} else {
				if payload == nil {
					t.Error("expected non-nil payload")
				} else {
					if !bytes.Equal(payload.Data, []byte(tc.responseBody)) {
						t.Errorf("payload data mismatch: expected %q, got %q", tc.responseBody, string(payload.Data))
					}
				}
			}
		})
	}
}

func TestRemoteClient_Put(t *testing.T) {
	testCases := []struct {
		name           string
		statusCode     int
		expectError    bool
		expectedErrMsg string
	}{
		{
			name:        "successful put",
			statusCode:  http.StatusOK,
			expectError: false,
		},
		{
			name:        "successful put with 201",
			statusCode:  http.StatusCreated,
			expectError: false,
		},
		{
			name:           "server error",
			statusCode:     http.StatusInternalServerError,
			expectError:    true,
			expectedErrMsg: "failed to put state",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testData := []byte(`{"version": 4, "terraform_version": "1.0.0"}`)
			
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "PUT" {
					t.Errorf("expected PUT request, got %s", r.Method)
				}
				
				// Verify headers
				if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
					t.Errorf("expected Authorization header 'Bearer test-token', got %q", auth)
				}
				
				if ct := r.Header.Get("Content-Type"); ct != "application/json" {
					t.Errorf("expected Content-Type 'application/json', got %q", ct)
				}
				
				if r.Header.Get("Content-MD5") == "" {
					t.Error("expected Content-MD5 header")
				}
				
				w.WriteHeader(tc.statusCode)
			}))
			defer server.Close()

			backend := &Backend{
				apiToken:   "test-token",
				httpClient: httpclient.New(context.Background()),
				endpoint:   server.URL,
			}

			client := &RemoteClient{
				backend:    backend,
				bucketName: "test-bucket",
				key:        "test.tfstate",
			}

			err := client.Put(context.Background(), testData)

			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if tc.expectedErrMsg != "" && !contains(err.Error(), tc.expectedErrMsg) {
					t.Errorf("expected error containing %q, got %v", tc.expectedErrMsg, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestRemoteClient_Delete(t *testing.T) {
	testCases := []struct {
		name           string
		statusCode     int
		expectError    bool
		expectedErrMsg string
	}{
		{
			name:        "successful delete 204",
			statusCode:  http.StatusNoContent,
			expectError: false,
		},
		{
			name:        "successful delete 200",
			statusCode:  http.StatusOK,
			expectError: false,
		},
		{
			name:        "not found is ok",
			statusCode:  http.StatusNotFound,
			expectError: false,
		},
		{
			name:           "server error",
			statusCode:     http.StatusInternalServerError,
			expectError:    true,
			expectedErrMsg: "failed to delete state",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "DELETE" {
					t.Errorf("expected DELETE request, got %s", r.Method)
				}
				
				// Verify authorization header
				if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
					t.Errorf("expected Authorization header 'Bearer test-token', got %q", auth)
				}
				
				w.WriteHeader(tc.statusCode)
			}))
			defer server.Close()

			backend := &Backend{
				apiToken:   "test-token",
				httpClient: httpclient.New(context.Background()),
				endpoint:   server.URL,
			}

			client := &RemoteClient{
				backend:    backend,
				bucketName: "test-bucket",
				key:        "test.tfstate",
			}

			err := client.Delete(context.Background())

			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if tc.expectedErrMsg != "" && !contains(err.Error(), tc.expectedErrMsg) {
					t.Errorf("expected error containing %q, got %v", tc.expectedErrMsg, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestRemoteClient_Lock(t *testing.T) {
	backend := &Backend{
		apiToken:   "test-token",
		httpClient: httpclient.New(context.Background()),
	}

	client := &RemoteClient{
		backend:    backend,
		bucketName: "test-bucket",
		key:        "test.tfstate",
	}

	// Lock should always succeed with a simple ID
	id, err := client.Lock(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id != "r2-no-lock" {
		t.Errorf("expected lock ID 'r2-no-lock', got %q", id)
	}
}

func TestRemoteClient_Unlock(t *testing.T) {
	backend := &Backend{
		apiToken:   "test-token",
		httpClient: httpclient.New(context.Background()),
	}

	client := &RemoteClient{
		backend:    backend,
		bucketName: "test-bucket",
		key:        "test.tfstate",
	}

	// Unlock should always succeed
	err := client.Unlock(context.Background(), "any-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoteClient_StateKey(t *testing.T) {
	testCases := []struct {
		name               string
		key                string
		workspaceKeyPrefix string
		workspace          string
		expected           string
	}{
		{
			name:               "default workspace",
			key:                "terraform.tfstate",
			workspaceKeyPrefix: "env:",
			workspace:          "default",
			expected:           "terraform.tfstate",
		},
		{
			name:               "named workspace",
			key:                "terraform.tfstate",
			workspaceKeyPrefix: "env:",
			workspace:          "production",
			expected:           "env:production/terraform.tfstate",
		},
		{
			name:               "custom prefix",
			key:                "state.tfstate",
			workspaceKeyPrefix: "workspaces/",
			workspace:          "staging",
			expected:           "workspaces/staging/state.tfstate",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			backend := &Backend{
				key:                tc.key,
				workspaceKeyPrefix: tc.workspaceKeyPrefix,
				encryption:         encryption.StateEncryptionDisabled(),
			}

			client, err := backend.remoteClient(tc.workspace)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if client.key != tc.expected {
				t.Errorf("expected key %q, got %q", tc.expected, client.key)
			}
		})
	}
}
