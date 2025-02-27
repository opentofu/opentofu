// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ociauthconfig

import (
	"context"
	"iter"
)

// CredentialsConfig is implemented by objects that can provide zero or more
// [CredentialsSource] objects given a registry domain and repository path.
//
// This package has its own implementaion of this interface in terms of a
// Docker CLI-style credentials configuration file, accessible through
// [FindDockerCLIStyleCredentialsConfigs] and [FixedDockerCLIStyleCredentialsConfigs],
// but package cliconfig also implements this separately for OpenTofu's own
// OCI credentials config language included as part of the OpenTofu CLI
// configuration.
type CredentialsConfig interface {
	CredentialsSourcesForRepository(ctx context.Context, registryDomain string, repositoryPath string) iter.Seq2[CredentialsSource, error]

	// CredentialsConfigLocationForUI returns a concise English-language
	// description of the config location for use in the UI, such as in
	// error messages if CredentialsSourcesForRepository returns an error.
	//
	// If possible, return an absolute filesystem path where the configuration
	// was loaded from, possibly followed by a colon and then a line number
	// within that file. Otherwise, a more general name for the location
	// is acceptable.
	CredentialsConfigLocationForUI() string
}

// NewGlobalDockerCredentialsHelperCredentialsConfig returns a [CredentialsConfig] that
// always returns exactly one [CredentialsSource] of specificity [GlobalCredentialsSpecificity]
// that associates the requested registry domain with the given Docker-style credential
// helper name.
func NewGlobalDockerCredentialHelperCredentialsConfig(locationForUI string, helperName string) CredentialsConfig {
	return globalDockerCredentialHelperCredentialsConfig{
		locationForUI: locationForUI,
		helperName:    helperName,
	}
}

type globalDockerCredentialHelperCredentialsConfig struct {
	locationForUI string
	helperName    string
}

// CredentialsConfigLocationForUI implements CredentialsConfig.
func (c globalDockerCredentialHelperCredentialsConfig) CredentialsConfigLocationForUI() string {
	return c.locationForUI
}

// CredentialsSourcesForRepository implements CredentialsConfig.
func (c globalDockerCredentialHelperCredentialsConfig) CredentialsSourcesForRepository(_ context.Context, registryDomain string, _ string) iter.Seq2[CredentialsSource, error] {
	// We just unconditionally associate the previously-configured credential helper
	// name with the domain that was requested. This source therefore serves as a
	// fallback for any repository address that isn't matched by any more-specific
	// credential sources coming from other configs.
	return func(yield func(CredentialsSource, error) bool) {
		yield(NewDockerCredentialHelperCredentialsSource(c.helperName, "https://"+registryDomain, GlobalCredentialsSpecificity), nil)
	}
}
