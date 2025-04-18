// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getmodules

import (
	"context"
	"io"

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// OCIRepositoryStore is the interface that [PackageFetcher] uses to
// interact with a single OCI Distribution repository when installing
// a remote module using the "oci" scheme.
//
// Implementations of this interface are returned by
// [PackageFetcherEnvironment.OCIRepositoryStore].
type OCIRepositoryStore interface {
	// Resolve finds the descriptor associated with the given tag name in the
	// repository, if any.
	//
	// tagName MUST conform to the pattern defined for "<reference> as a tag"
	// from the OCI distribution specification, or the result is unspecified.
	Resolve(ctx context.Context, tagName string) (ociv1.Descriptor, error)

	// Fetch retrieves the content of a specific blob from the repository, identified
	// by the digest in the given descriptor.
	//
	// Implementations of this function tend not to check that the returned content
	// actually matches the digest and size in the descriptor, so callers MUST
	// verify that somehow themselves before making use of the resulting content.
	//
	// Callers MUST close the returned reader after using it, since it's typically
	// connected to an active network socket or file handle.
	Fetch(ctx context.Context, target ociv1.Descriptor) (io.ReadCloser, error)

	// The design of the above intentionally matches a subset of the interfaces
	// defined in the ORAS-Go library, but we have our own specific interface here
	// both to clearly define the minimal interface we depend on and so that our
	// use of ORAS-Go can be treated as an implementation detail rather than as
	// an API contract should we need to switch to a different approach in future.
	//
	// If you need to expand this while we're still relying on ORAS-Go, aim to
	// match the corresponding interface in that library if at all possible so
	// that we can minimize the amount of adapter code we need to write.
}
