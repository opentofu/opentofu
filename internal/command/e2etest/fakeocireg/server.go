// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package fakeocireg

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"

	orasOCI "oras.land/oras-go/v2/content/oci"
)

// NewServer returns a preconfigured and started [httptest.Server] that
// supports some read-only parts of the OCI Distribution protocol by
// interacting with the OCI "Image Layouts" in the directories specified
// as values of localRepoDirs. The keys of that map must be valid OCI
// repository names.
//
// This specification defines the expected content of the given directory:
//
//	https://github.com/opencontainers/image-spec/blob/v1.1.1/image-layout.md
//
// Currently this implementation offers a repository that does not require
// any authentication at all, so it's useful for testing interactions with
// the Distribution protocol and repository contents, but not for testing
// authentication support. For now we're assuming that the auth support is
// tested well enough in the relevant unit tests in other packages.
//
// The caller must treat the result in all of the same ways required for
// a direct call to [httptest.NewTLSServer], because this is really just a
// wrapper around that function but with a handler implementation provided
// within this package.
func NewServer(ctx context.Context, localRepoDirs map[string]string) (*httptest.Server, error) {
	stores := make(map[string]*orasOCI.ReadOnlyStore, len(localRepoDirs))
	for repoName, dir := range localRepoDirs {
		fsys := os.DirFS(dir)
		store, err := orasOCI.NewFromFS(ctx, fsys)
		if err != nil {
			return nil, fmt.Errorf("failed to open OCI layout for %s: %w", repoName, err)
		}
		stores[repoName] = store
	}
	return httptest.NewTLSServer(registryHandler{stores}), nil
}
