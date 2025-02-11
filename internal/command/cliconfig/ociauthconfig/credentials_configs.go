// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ociauthconfig

import (
	"context"
	"errors"
	"fmt"
)

// CredentialsConfigs represents a sequence of [CredentialsConfig] objects that
// can be queried together to find credentials for making requests to a specific
// OCI repository.
type CredentialsConfigs struct {
	configs []CredentialsConfig
}

// Creates a new [CredentialsConfigs] object with the given sequence of credentials
// configurations.
//
// The caller must not modify the backing array of the given slice, or anything
// accessible through it, after passing it to this function.
func NewCredentialsConfigs(configs []CredentialsConfig) CredentialsConfigs {
	return CredentialsConfigs{configs}
}

// CredentialsSourceForRepository finds the most specifically-configured credentials source
// for the given registry domain and repository path, if any.
//
// If a successful search across all configuration objects finds no credential sources that
// match the given registry domain and repository path then the error result is one that
// would cause [IsCredentialsNotFoundError] to return true. If the returned error is nil
// then the [CredentialsSource] result is guaranteed to be non-nil.
//
// If multiple credentials sources match with equal specificity then the result is the first
// one encountered with that specificity, and so earlier credentials configurations take
// precedence over later ones.
//
// The result is a [CredentialsSource] rather than a [Credentials] directly so that the
// caller can separate the step of analyzing the config from the step of actually fetching
// the credentials, since fetching the credentials might involve executing an external
// program if the most specific available source is a credential helper.
func (s *CredentialsConfigs) CredentialsSourceForRepository(ctx context.Context, registryDomain, repositoryPath string) (CredentialsSource, error) {
	var err error
	var resultSource CredentialsSource
	resultSpec := NoCredentialsSpecificity
	for _, config := range s.configs {
		for thisSource, sourceErr := range config.CredentialsSourcesForRepository(ctx, registryDomain, repositoryPath) {
			if sourceErr != nil {
				if !IsCredentialsNotFoundError(sourceErr) {
					err = errors.Join(err, fmt.Errorf("from %s: %w", config.CredentialsConfigLocationForUI(), sourceErr))
				}
				continue
			}
			thisSpec := thisSource.CredentialsSpecificity()
			if thisSpec <= resultSpec {
				continue // We only consider sources with higher specificity than the one we previously selected
			}
			resultSource = thisSource
			resultSpec = thisSpec
		}
	}
	if resultSpec == NoCredentialsSpecificity {
		err = NewCredentialsNotFoundError(fmt.Errorf("no credentials configured for %s/%s", registryDomain, repositoryPath))
	}
	return resultSource, err
}

// AllConfigs returns the full set of configurations that this [CredentialsConfigs]
// object will consult when asked for credentials, in the order that they would
// be consulted.
//
// The caller must not modify anything accessible through the returned slice.
func (s *CredentialsConfigs) AllConfigs() []CredentialsConfig {
	return s.configs
}
