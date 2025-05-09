// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"

	"github.com/opentofu/opentofu/internal/getmodules"
)

func remoteModulePackageFetcher(ctx context.Context, getOCICredsPolicy ociCredsPolicyBuilder) *getmodules.PackageFetcher {
	// TODO: Pass in a real getmodules.PackageFetcherEnvironment here,
	// which knows how to make use of the OCI authentication policy.
	return getmodules.NewPackageFetcher(ctx, &modulePackageFetcherEnvironment{
		getOCICredsPolicy: getOCICredsPolicy,
	})
}

type modulePackageFetcherEnvironment struct {
	getOCICredsPolicy ociCredsPolicyBuilder
}

// OCIRepositoryStore implements getmodules.PackageFetcherEnvironment.
func (m *modulePackageFetcherEnvironment) OCIRepositoryStore(ctx context.Context, registryDomainName string, repositoryPath string) (getmodules.OCIRepositoryStore, error) {
	// We intentionally delay the finalization of the credentials policy until
	// just before we need it because most OpenTofu commands don't install
	// module packages at all, and even those that do only need to do this if
	// using the "oci" source type, so we can avoid doing this work at all
	// most of the time.
	credsPolicy, err := m.getOCICredsPolicy(ctx)
	if err != nil {
		// This deals with only a small number of errors that we can't catch during CLI config validation
		return nil, fmt.Errorf("invalid credentials configuration for OCI registries: %w", err)
	}
	return getOCIRepositoryStore(ctx, registryDomainName, repositoryPath, credsPolicy)
}
