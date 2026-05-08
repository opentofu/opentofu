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
	"github.com/opentofu/opentofu/internal/command/e2etest/fakeocireg"
	orasRemote "oras.land/oras-go/v2/registry/remote"
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

// Test that getOCIRepositoryORASClient uses the correct credentials for each repository
// when a registry has multiple repositories with different credentials.
func TestGetOCIRepositoryORASClient_PerRepositoryCredentials(t *testing.T) {
	// Configure two repositories with different credentials for same registry
	credsPolicy := ociauthconfig.NewCredentialsConfigs([]ociauthconfig.CredentialsConfig{
		&testPerRepoCredentialsConfig{
			domain: "registry.example.com",
			repos: map[string]ociauthconfig.Credentials{
				"repo-a": ociauthconfig.NewBasicAuthCredentials("user-a", "password-a"),
				"repo-b": ociauthconfig.NewBasicAuthCredentials("user-b", "password-b"),
			},
		},
	})

	for _, id := range [...]string{"a", "b"} {
		t.Run(id, func(t *testing.T) {
			ctx := t.Context()
			client, err := getOCIRepositoryORASClient(ctx, "registry.example.com", "repo-"+id, credsPolicy)

			if err != nil {
				t.Fatalf("failed creating credential: %s", err)
			}

			credentials, err := client.Credential(ctx, "registry.example.com")
			if err != nil {
				t.Fatalf("failed resolving credential: %s", err)
			}

			want, got := "user-"+id, credentials.Username
			if want != got {
				t.Fatalf("username mismatch: %s != %s", got, want)
			}

			want, got = "password-"+id, credentials.Password
			if want != got {
				t.Fatalf("password mismatch: %s != %s", got, want)
			}

		})
	}
}

// TestGetOciRepositoryNoPing verifies that getOCIRepositoryStore can successfully
// connect to an OCI registry that returns a 401 Unauthorized response for
// the `GET /v2/` endpoint, which is a behavior seen with some registries.
//
// This test ensures that OpenTofu does not use the problematic `oras.Ping()`
// function, which would fail against such registries. Instead, authentication
// is handled transparently by ORAS on the first real API call.
func TestGetOciRepositoryNoPing(t *testing.T) {
	ctx := t.Context()

	// No authentication, empty credentials
	credsPolicy := ociauthconfig.NewCredentialsConfigs(nil)

	registryServer, err := fakeocireg.NewServer(t.Context(), map[string]string{	})
	if err != nil {
		t.Fatal(err)
	}
	defer registryServer.Close()

	registryAddr := registryServer.Listener.Addr().String()
	repositoryName := "repo-a"

	store, err := getOCIRepositoryStore(ctx, registryAddr, repositoryName, credsPolicy)

	if err != nil {
		t.Fatalf("failed creating oci repository store: %s", err)
	}

	if store == nil {
		t.Fatal("failed creating oci repository store, expected non-nil store")
	}

	repo, ok := store.(*orasRemote.Repository)
	if !ok {
		t.Fatal("failed creating oci repository store, expected orasRemote.Repository")
	}

	if repo.Reference.Registry != registryAddr {
		t.Errorf("wrong registry; got %q, want %q", repo.Reference.Registry, registryAddr)
	}

	if repo.Reference.Repository != repositoryName {
		t.Errorf("wrong repository name; got %q, want %q", repo.Reference.Repository, repositoryName)
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
