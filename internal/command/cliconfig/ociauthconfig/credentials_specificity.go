// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ociauthconfig

import (
	"fmt"
	"math"
)

// CredentialsSpecificity is an approximation of the "specificity" of a set of
// credentials, used to help a [CredentialsConfigs] to select the most specific
// credentials returned from a number of different [CredentialsConfig]
// implementations.
//
// This is represented as a uint for convenient use with Go's numeric comparison
// operators -- a greater value represents greater specificity -- but callers
// should not otherwise depend on its representation directly and should instead
// use the exported functions and methods to interact with values of this type.
type CredentialsSpecificity uint

// NoCredentialsSpecificity is the zero value of CredentialsSpecificity,
// representing the total absense of a specificity value.
const NoCredentialsSpecificity = CredentialsSpecificity(0)

// GlobalCredentialsSpecificity is the lowest level of specificity, used for
// credentials that are defined globally, rather than domain-specific or
// repository-specific.
const GlobalCredentialsSpecificity = CredentialsSpecificity(1)

// DomainCredentialsSpecificity is the second-lowest level of specificity, used
// for credentials that are associated with an entire domain.
const DomainCredentialsSpecificity = CredentialsSpecificity(2)

// RepositoryCredentialsSpecificity returns a CredentialsSpecificity for a
// repository path with a given number of path segments.
//
// If pathSegments is zero then the result is [DomainCredentialsSpecificity],
// since that represents just a domain match without any path segment matches.
func RepositoryCredentialsSpecificity(pathSegments uint) CredentialsSpecificity {
	const offset = uint(DomainCredentialsSpecificity)
	if pathSegments > (math.MaxUint - offset) {
		// We saturate at the maximum uint since a path with that many segments
		// would be pretty ridiculous and so not realistic in practice.
		return CredentialsSpecificity(math.MaxUint)
	}
	return CredentialsSpecificity(pathSegments + offset)
}

// MatchedRegistryDomain returns true if the specificity covers at least a specific
// registry domain.
func (s CredentialsSpecificity) MatchedRegistryDomain() bool {
	return uint(s) >= uint(DomainCredentialsSpecificity)
}

// MatchedRepositoryPath returns true if the specificity covers at least a specific
// registry domain and at least one path segment.
func (s CredentialsSpecificity) MatchedRepositoryPath() bool {
	return s.MatchedRepositoryPathSegments() > 0
}

// MatchedRepositoryPathSegments returns the number of segments at the start of
// the repository path matched with the credentials source this specificity
// is describing.
//
// Returns zero if the credentials source is not specific to any path.
func (s CredentialsSpecificity) MatchedRepositoryPathSegments() uint {
	const offset = uint(DomainCredentialsSpecificity)
	if uint(s) < offset {
		return 0
	}
	return uint(s) - offset
}

func (s CredentialsSpecificity) GoString() string {
	switch s {
	case NoCredentialsSpecificity:
		return "ociauthconfig.NoCredentialsSpecificity"
	case GlobalCredentialsSpecificity:
		return "ociauthconfig.GlobalCredentialsSpecificity"
	case DomainCredentialsSpecificity:
		return "ociauthconfig.DomainCredentialsSpecificity"
	default:
		pathSegCount := s.MatchedRepositoryPathSegments()
		return fmt.Sprintf("ociauthconfig.RepositoryCredentialsSpecificity(%d)", pathSegCount)
	}
}
