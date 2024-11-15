// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"fmt"

	ociDigest "github.com/opencontainers/go-digest"
)

// PackageOCIObject refers to an object in an OCI repository that is to be
// treated as a provider package.
//
// The manifest associated with the given digest should be a single
// image manifest for a specific platform. It should _not_ be a multi-platform
// manifest, because the decision about which platform to select should
// have already been made by whatever generates an object of this type.
type PackageOCIObject struct {
	// RegistryHostname is the hostname of the registry that hosts the
	// repository, including an optional port number appended after
	// a colon.
	RegistryHostname string

	// RepositoryName is the name of the repository hosted on RegistryHostname,
	// which is assumed to conform to the following pattern defined in the
	// OCI distribution specification:
	//    [a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*(\/[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*)*
	RepositoryName string

	// ManifestDigest is the digest of the manifest describing the blobs
	// required to retrieve and reconstitute the single object that is to
	// be used as a provider package.
	ManifestDigest ociDigest.Digest
}

var _ PackageLocation = PackageOCIObject{}

func (p PackageOCIObject) String() string {
	// The following is intended to mimic the typical shorthand syntax for
	// referring to a specific image from an OCI repository, yielding
	// something like this:
	//     example.com/foo/bar@sha256:2e863c44b718727c850746562e1d54afd13b2fa71b160f5cd9058fc436217c30
	return fmt.Sprintf("%s/%s@%s", p.RegistryHostname, p.RepositoryName, p.ManifestDigest)
}

func (p PackageOCIObject) InstallProviderPackage(_ context.Context, _ PackageMeta, _ string, _ []Hash) (*PackageAuthenticationResult, error) {
	// TODO: Implement
	return nil, fmt.Errorf("installing OCI distribution objects as provider packages is not yet implemented")
}
