// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ociauthconfig

import (
	"context"
	"fmt"
	"io/fs"
	"testing"
)

type fakeConfigDiscoveryEnvironment struct {
	envVars  map[string]string
	homePath string
	osName   string
	files    map[string][]byte

	t *testing.T
}

var _ ConfigDiscoveryEnvironment = (*fakeConfigDiscoveryEnvironment)(nil)

// EnvironmentVariableVal implements ConfigDiscoveryEnvironment.
func (f *fakeConfigDiscoveryEnvironment) EnvironmentVariableVal(name string) string {
	return f.envVars[name]
}

// OperatingSystemName implements ConfigDiscoveryEnvironment.
func (f *fakeConfigDiscoveryEnvironment) OperatingSystemName() string {
	return f.osName
}

// ReadFile implements ConfigDiscoveryEnvironment.
func (f *fakeConfigDiscoveryEnvironment) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if f.t != nil {
		f.t.Helper()
		f.t.Logf("fakeConfigDiscoveryEnvironment: reading %q", path)
	}
	ret, ok := f.files[normalizeFilePath(path)]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return ret, nil
}

// UserHomeDirPath implements ConfigDiscoveryEnvironment.
func (f *fakeConfigDiscoveryEnvironment) UserHomeDirPath() string {
	return f.homePath
}

type fakeCredentialsLookupEnvironment struct {
	credentialsHelperResults map[string]map[string]DockerCredentialHelperGetResult

	t *testing.T
}

var _ CredentialsLookupEnvironment = (*fakeCredentialsLookupEnvironment)(nil)

// QueryDockerCredentialHelper implements CredentialsLookupEnvironment.
func (f *fakeCredentialsLookupEnvironment) QueryDockerCredentialHelper(ctx context.Context, helperName string, serverURL string) (DockerCredentialHelperGetResult, error) {
	if f.t != nil {
		f.t.Helper()
		f.t.Logf("fakeCredentialsLookupEnvironment: querying helper %q with server URL %q", helperName, serverURL)
	}
	helperResults, ok := f.credentialsHelperResults[helperName]
	if !ok {
		return DockerCredentialHelperGetResult{}, NewCredentialsNotFoundError(fmt.Errorf("no fake results for helper %q", helperName))
	}
	result, ok := helperResults[serverURL]
	if !ok {
		return DockerCredentialHelperGetResult{}, NewCredentialsNotFoundError(fmt.Errorf("fake results for %q do not have an entry for %q", helperName, serverURL))
	}
	if result.ServerURL == "" {
		// We'll save the test author the trouble of hand-writing a
		// suitable ServerURL since we know what we were asked for.
		// The following is okay because result is a copy of the object from
		// the credentialsHelperResults map, rather than a pointer.
		result.ServerURL = serverURL
	}
	return result, nil
}
