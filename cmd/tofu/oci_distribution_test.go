// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"os"
	"strings"
	"testing"
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
