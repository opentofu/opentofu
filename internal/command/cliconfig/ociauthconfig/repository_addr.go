// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ociauthconfig

import (
	"fmt"
	"strings"

	orasregistry "oras.land/oras-go/v2/registry"
)

// ParseRepositoryAddressPrefix attempts to parse the given string as the compact
// "registry-domain/repository-path" format, returning the separate registry domain
// and repository path parts if it's valid.
//
// This function is intended for the repository address prefix syntax used for
// specifying "auths" in the Docker CLI-style config format, and for OpenTofu's
// own oci_credentials CLI configuration blocks that follow the same syntax.
//
// The registry-domain portion can optionally include a port number delimited
// by a colon, like "localhost:5000". The repository-path portion is optional,
// with an address without it referring to all repositories on the given
// registry domain.
func ParseRepositoryAddressPrefix(addr string) (registryDomain, repositoryPath string, err error) {
	// For now we're relying on the ORAS implementation of parsing, but if we decide to
	// move away from ORAS for other registry client purposes then we can either
	// implement something similar inline here or find an alternative external library
	// to use for this.
	// We're actually using the _reference_ parser here, since a reference incorporates
	// a repository address, but we'll reject after the fact any result that includes
	// a tag or digest portion since we're not intending to accept addresses of specific
	// artifacts.

	if strings.Count(addr, "/") != 0 {
		// This seems to be an address with both a registry and a repository path.
		ref, parseErr := orasregistry.ParseReference(addr)
		if parseErr != nil {
			// The ORAS function returns errors with sufficient context that any
			// further decoration we might add here would be redundant. For example,
			// this might return an error whose message is
			// "invalid reference: invalid registry invalid:thing:blah".
			return "", "", parseErr
		}
		if ref.Reference != "" {
			return "", "", fmt.Errorf("invalid reference: artifact tag or digest not allowed")
		}
		return ref.Registry, ref.Repository, nil
	}

	// If we get here then it seems like we have _just_ a domain part. ORAS does
	// not have a separate function just for parsing a domain, so we'll borrow the
	// validate function from its reference parser instead.
	ref := &orasregistry.Reference{
		Registry: addr,
	}
	err = ref.ValidateRegistry()
	// ValidateRegistry returns an error with a string like "invalid reference: invalid registry invalid:thing:blah"
	return addr, "", err
}
