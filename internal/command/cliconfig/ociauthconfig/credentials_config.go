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
