// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ociauthconfig

import (
	"context"
)

// ConfigDiscoveryEnvironment is the dependency-inversion adapter used when
// attempting to discover configuration files.
//
// Implementers of this interface expose information about the environment
// in which OpenTofu is running, which can potentially affect which
// configuration files are found and selected.
type ConfigDiscoveryEnvironment interface {
	// EnvironmentVariableVal returns the value of the environment variable
	// with the given name. Returns an empty string if either the variable
	// is defined as having an empty string value or it is not defined at all.
	EnvironmentVariableVal(name string) string

	// UserHomeDirPath returns a path that should be treated as the home
	// directory for the user that is running the program.
	UserHomeDirPath() string

	// OperatingSystemName returns the [runtime.GOOS]-style operating system
	// name for the operating system where the program is running, or (in
	// tests) the operating system where we're _pretending_ that the program
	// is running.
	//
	// The implementation in this package aims to follow the conventions of
	// the OS reported by this method instead of the OS where the program
	// is running in cases where those differ, but they should differ only
	// in tests so that promise is kept only to the extent that portable
	// unit tests might benefit from it. (We want to be able to test how
	// this wouuld behave on several different platforms regardless of where
	// the tests are actually running.)
	OperatingSystemName() string

	// ReadFile returns the raw content of the file at the given path, if
	// possible.
	//
	// Returns an error that would cause [os.IsNotExist] to return true if
	// the requested file does not exist.
	ReadFile(ctx context.Context, path string) ([]byte, error)
}

// CredentialsLookupEnvironment is the dependency-inversion adapter used
// when attempting to use a [CredentialsSource] to obtain some actual
// [Credentials].
//
// Package ociauthconfig is focused on policy rather than mechanism, so
// implementers of this interface define the mechanism by which the
// credentials sources can access their surrounding environment.
type CredentialsLookupEnvironment interface {
	// QueryDockerCredentialHelper performs a "get" request to the Docker
	// credential helper whose name is given in helperName, asking for
	// credentials for the given server URL.
	//
	// If the credential helper indicates that the request is valid but
	// there are no credentials available for the given server URL then
	// the error result is something that would cause
	// [IsCredentialsNotFoundError] to return true.
	QueryDockerCredentialHelper(ctx context.Context, helperName string, serverURL string) (DockerCredentialHelperGetResult, error)
}
