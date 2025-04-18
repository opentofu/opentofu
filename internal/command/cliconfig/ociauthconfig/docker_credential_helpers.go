// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ociauthconfig

// DockerCredentialHelperResult represents the result of a "get" request to
// a Docker-style credentials helper, as described in
// https://github.com/docker/docker-credential-helpers .
type DockerCredentialHelperGetResult struct {
	ServerURL        string
	Username, Secret string
}
