// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"fmt"

	"github.com/opentofu/libregistry/registryprotocols/ociclient"
)

// PackageOCIObject refers to an object in an OCI repository that is to be
// treated as a provider package.
//
// The manifest associated with the given digest should be a single
// image manifest for a specific platform. It should _not_ be a multi-platform
// manifest, because the decision about which platform to select should
// have already been made by whatever generates an object of this type.
type PackageOCIObject struct {
	// imageMetadata describes both the address of the specific object that's
	// being installed and the digests of all of the blobs the serve as its
	// layers.
	imageMetadata ociclient.OCIImageMetadata

	// client is the OCI client that should be used to retrieve the
	// object's layers.
	client ociclient.OCIClient
}

var _ PackageLocation = PackageOCIObject{}

func (p PackageOCIObject) String() string {
	return p.imageMetadata.Addr.String()
}

func (p PackageOCIObject) InstallProviderPackage(_ context.Context, _ PackageMeta, _ string, _ []Hash) (*PackageAuthenticationResult, error) {
	// TODO: Implement
	return nil, fmt.Errorf("installing OCI distribution objects as provider packages is not yet implemented")
}
