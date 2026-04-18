// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"iter"
	"os"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/command/cliconfig/ociauthconfig"
)

func TestOCICredentialsLookupEnv_DockerCredHelper(t *testing.T) {
	// This ociCredentialsLookupEnv is the concrete implementation of
	// the abstraction that is created in package ociauthconfig, and
	// so it depends directly on a real (non-substitutable)
	// implementation of Docker credential helpers.
	//
	// Properly testing this would require a functioning credential
	// helper executable, which is difficult to arrange for in a
	// portable manner to allow this test to run across multiple
	// platforms. It's just a thin wrapper around the ORAS-Go
	// implementation anyway, and that has its own tests upstream
	// so this test just settles for the compromise of making a
	// call that we expect to fail and checking that it fails in
	// the way we expect, to give confidence that we really did
	// ask the ORAS-Go library to attempt to fetch credentials.

	// To prevent this from accidentally executing some real
	// credential helper that might be installed on the system
	// where the tests are running, we'll temporarily override
	// the PATH environment variable to include only an empty
	// directory.
	emptyDir := os.TempDir()
	os.Setenv("PATH", emptyDir)

	env := ociCredentialsLookupEnv{}
	_, err := env.QueryDockerCredentialHelper(t.Context(), "fake-for-testing", "https://example.com")

	if err == nil {
		t.Fatal("unexpected success; want error")
	}

	// The exact details of the error message can vary between
	// platforms, but it should always mention that it was
	// trying to execute the specified credential helper
	// executable.
	wantErr := `docker-credential-fake-for-testing`
	if gotErr := err.Error(); !strings.Contains(gotErr, wantErr) {
		t.Errorf("wrong error\ngot: %s\nwant substring: %s", gotErr, wantErr)
	}
}

// TestGetOCIRepositoryORASClient_PerRepositoryCredentials verifies that when a
// single registry hosts two repositories each requiring different credentials,
// getOCIRepositoryORASClient selects the correct credential source for each
// repository independently — i.e. credentials are not shared or mixed up across
// repositories on the same host.
//
// This addresses the reviewer concern from the PR: if registry.example.com/repo-a
// and registry.example.com/repo-b each need different credentials, both can be
// used in the same "tofu init" run and each will receive the right credentials.
func TestGetOCIRepositoryORASClient_PerRepositoryCredentials(t *testing.T) {
	ctx := t.Context()

	// Configure two distinct sets of credentials scoped to separate repositories
	// on the same registry domain.
	credsPolicy := ociauthconfig.NewCredentialsConfigs([]ociauthconfig.CredentialsConfig{
		&testPerRepoCredentialsConfig{
			domain: "registry.example.com",
			repos: map[string]ociauthconfig.Credentials{
				"repo-a": ociauthconfig.NewBasicAuthCredentials("user-a", "secret-a"),
				"repo-b": ociauthconfig.NewBasicAuthCredentials("user-b", "secret-b"),
			},
		},
	})

	// Create a separate ORAS client for each repository.
	// getOCIRepositoryORASClient resolves and closes over the credential source
	// at construction time, so each client is independent.
	clientA, err := getOCIRepositoryORASClient(ctx, "registry.example.com", "repo-a", credsPolicy)
	if err != nil {
		t.Fatalf("creating client for repo-a: %s", err)
	}
	clientB, err := getOCIRepositoryORASClient(ctx, "registry.example.com", "repo-b", credsPolicy)
	if err != nil {
		t.Fatalf("creating client for repo-b: %s", err)
	}

	// Invoke the Credential callback directly — this is what ORAS calls when it
	// needs to authenticate an outgoing request.
	credA, err := clientA.Credential(ctx, "registry.example.com")
	if err != nil {
		t.Fatalf("resolving credential for repo-a: %s", err)
	}
	if got, want := credA.Username, "user-a"; got != want {
		t.Errorf("repo-a: wrong username: got %q, want %q", got, want)
	}
	if got, want := credA.Password, "secret-a"; got != want {
		t.Errorf("repo-a: wrong password: got %q, want %q", got, want)
	}

	credB, err := clientB.Credential(ctx, "registry.example.com")
	if err != nil {
		t.Fatalf("resolving credential for repo-b: %s", err)
	}
	if got, want := credB.Username, "user-b"; got != want {
		t.Errorf("repo-b: wrong username: got %q, want %q", got, want)
	}
	if got, want := credB.Password, "secret-b"; got != want {
		t.Errorf("repo-b: wrong password: got %q, want %q", got, want)
	}
}

// testPerRepoCredentialsConfig is a test-only implementation of
// [ociauthconfig.CredentialsConfig] that maps repository paths within a fixed
// domain to static credentials.
type testPerRepoCredentialsConfig struct {
	domain string
	repos  map[string]ociauthconfig.Credentials
}

func (c *testPerRepoCredentialsConfig) CredentialsSourcesForRepository(_ context.Context, registryDomain, repositoryPath string) iter.Seq2[ociauthconfig.CredentialsSource, error] {
	return func(yield func(ociauthconfig.CredentialsSource, error) bool) {
		if registryDomain != c.domain {
			return
		}
		creds, ok := c.repos[repositoryPath]
		if !ok {
			return
		}
		yield(ociauthconfig.NewStaticCredentialsSource(creds, ociauthconfig.RepositoryCredentialsSpecificity(1)), nil)
	}
}

func (c *testPerRepoCredentialsConfig) CredentialsConfigLocationForUI() string {
	return "test fixture"
}
