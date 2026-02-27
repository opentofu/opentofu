// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package oras

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	orasAuth "oras.land/oras-go/v2/registry/remote/auth"
)

func TestParseGHCRRepository(t *testing.T) {
	tests := []struct {
		name        string
		repository  string
		wantHost    string
		wantOwner   string
		wantPackage string
		wantErr     bool
	}{
		{
			name:        "valid ghcr.io repository",
			repository:  "ghcr.io/myorg/tofu-state",
			wantHost:    "ghcr.io",
			wantOwner:   "myorg",
			wantPackage: "tofu-state",
		},
		{
			name:        "valid ghcr.io with nested path",
			repository:  "ghcr.io/myorg/infra/prod/state",
			wantHost:    "ghcr.io",
			wantOwner:   "myorg",
			wantPackage: "infra/prod/state",
		},
		{
			name:        "non-ghcr registry",
			repository:  "ecr.aws/myorg/tofu-state",
			wantHost:    "ecr.aws",
			wantOwner:   "myorg",
			wantPackage: "tofu-state",
		},
		{
			name:       "too few segments",
			repository: "ghcr.io/myorg",
			wantErr:    true,
		},
		{
			name:       "single segment",
			repository: "ghcr.io",
			wantErr:    true,
		},
		{
			name:       "empty string",
			repository: "",
			wantErr:    true,
		},
		{
			name:       "empty host segment",
			repository: "/myorg/state",
			wantErr:    true,
		},
		{
			name:       "empty owner segment",
			repository: "ghcr.io//state",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, owner, pkg, err := parseGHCRRepository(tt.repository)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if host != tt.wantHost {
				t.Fatalf("host = %q, want %q", host, tt.wantHost)
			}
			if owner != tt.wantOwner {
				t.Fatalf("owner = %q, want %q", owner, tt.wantOwner)
			}
			if pkg != tt.wantPackage {
				t.Fatalf("package = %q, want %q", pkg, tt.wantPackage)
			}
		})
	}
}

func TestTryDeleteGHCRTag_NilRepo(t *testing.T) {
	err := tryDeleteGHCRTag(context.Background(), nil, "test-tag")
	if err == nil {
		t.Fatalf("expected error for nil repo")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Fatalf("expected nil-related error, got: %v", err)
	}
}

func TestTryDeleteGHCRTag_NotGHCR(t *testing.T) {
	repo := &orasRepositoryClient{
		repository: "ecr.aws/myorg/state",
		inner:      newFakeORASRepo(),
	}
	err := tryDeleteGHCRTag(context.Background(), repo, "test-tag")
	if err != errNotGHCR {
		t.Fatalf("expected errNotGHCR, got: %v", err)
	}
}

func TestTryDeleteGHCRTag_NoCredentials(t *testing.T) {
	repo := &orasRepositoryClient{
		repository: "ghcr.io/myorg/state",
		inner:      newFakeORASRepo(),
		// authFn is nil — no credentials available
	}
	err := tryDeleteGHCRTag(context.Background(), repo, "test-tag")
	if err == nil {
		t.Fatalf("expected error for missing credentials")
	}
	if !strings.Contains(err.Error(), "no credentials") {
		t.Fatalf("expected 'no credentials' error, got: %v", err)
	}
}

func TestDeleteGitHubPackageVersionByTag_OrgEndpoint(t *testing.T) {
	const targetTag = "state-default-v1"
	const versionID int64 = 42

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/orgs/myorg/packages/container/state/versions"):
			// Return a list with one version matching the target tag.
			versions := []githubPackageVersion{
				{
					ID: versionID,
					Metadata: struct {
						Container struct {
							Tags []string `json:"tags"`
						} `json:"container"`
					}{
						Container: struct {
							Tags []string `json:"tags"`
						}{
							Tags: []string{targetTag, "other-tag"},
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(versions)

		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, fmt.Sprintf("/versions/%d", versionID)):
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := srv.Client()
	err := deleteGitHubPackageVersionByTag(context.Background(), client, srv.URL, "myorg", "state", targetTag, "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteGitHubPackageVersionByTag_FallsBackToUserEndpoint(t *testing.T) {
	const targetTag = "state-default-v1"
	const versionID int64 = 99

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		// Org endpoint returns 404
		case strings.Contains(r.URL.Path, "/orgs/"):
			http.Error(w, "not found", http.StatusNotFound)

		// User endpoint: list versions
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/users/myuser/packages/container/state/versions"):
			versions := []githubPackageVersion{
				{
					ID: versionID,
					Metadata: struct {
						Container struct {
							Tags []string `json:"tags"`
						} `json:"container"`
					}{
						Container: struct {
							Tags []string `json:"tags"`
						}{
							Tags: []string{targetTag},
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(versions)

		// User endpoint: delete
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, fmt.Sprintf("/versions/%d", versionID)):
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	client := srv.Client()
	err := deleteGitHubPackageVersionByTag(context.Background(), client, srv.URL, "myuser", "state", targetTag, "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteGitHubPackageVersionByTag_TagNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/versions") {
			// Return an empty list — tag doesn't exist.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]githubPackageVersion{})
			return
		}
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := srv.Client()
	// Both org and user endpoints return empty lists → tag not found.
	err := deleteGitHubPackageVersionByTag(context.Background(), client, srv.URL, "myorg", "state", "nonexistent", "test-token")
	if err == nil {
		t.Fatalf("expected error for missing tag")
	}
	if !isHTTPStatus(err, http.StatusNotFound) {
		t.Fatalf("expected 404 status error, got: %v", err)
	}
}

func TestIsHTTPStatus(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		code     int
		expected bool
	}{
		{name: "matching status", err: newHTTPStatusError(404, "test"), code: 404, expected: true},
		{name: "non-matching status", err: newHTTPStatusError(404, "test"), code: 500, expected: false},
		{name: "non-http error", err: fmt.Errorf("generic error"), code: 404, expected: false},
		{name: "nil error", err: nil, code: 404, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isHTTPStatus(tt.err, tt.code); got != tt.expected {
				t.Fatalf("isHTTPStatus(%v, %d) = %v, want %v", tt.err, tt.code, got, tt.expected)
			}
		})
	}
}

func TestTryDeleteGHCRTag_Integration(t *testing.T) {
	// Integration-style test using a fake GitHub API server and a fake OCI repo.
	const targetTag = "locked-default"
	const versionID int64 = 7

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/orgs/myorg/packages/container/state/versions"):
			versions := []githubPackageVersion{
				{
					ID: versionID,
					Metadata: struct {
						Container struct {
							Tags []string `json:"tags"`
						} `json:"container"`
					}{
						Container: struct {
							Tags []string `json:"tags"`
						}{
							Tags: []string{targetTag},
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(versions)

		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, fmt.Sprintf("/versions/%d", versionID)):
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	// We can't call tryDeleteGHCRTag directly with a custom base URL, so we
	// test deleteGitHubPackageVersionByTag which is the core logic.
	client := srv.Client()
	err := deleteGitHubPackageVersionByTag(context.Background(), client, srv.URL, "myorg", "state", targetTag, "my-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTryDeleteGHCRTag_UsesRepoHTTPClient(t *testing.T) {
	// Test that tryDeleteGHCRTag uses repo.httpClient when available.
	// We set up a repo with a custom httpClient that returns a predictable error
	// and verify that error comes through.

	repo := &orasRepositoryClient{
		repository: "ghcr.io/myorg/state",
		inner:      newFakeORASRepo(),
		httpClient: &http.Client{
			Transport: &failingTransport{err: fmt.Errorf("custom transport error")},
		},
		authFn: func(ctx context.Context, hostport string) (orasAuth.Credential, error) {
			return orasAuth.Credential{Password: "test-token"}, nil
		},
	}

	err := tryDeleteGHCRTag(context.Background(), repo, "test-tag")
	if err == nil {
		t.Fatalf("expected error from custom transport")
	}
	// The error should come from our custom transport, not from trying to create a new client.
	if !strings.Contains(err.Error(), "custom transport error") {
		t.Fatalf("expected custom transport error, got: %v", err)
	}
}

// failingTransport always returns the configured error.
type failingTransport struct {
	err error
}

func (t *failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, t.err
}
