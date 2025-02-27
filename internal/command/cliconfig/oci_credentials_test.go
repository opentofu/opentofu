// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestLoadConfig_ociDefaultCredentials(t *testing.T) {
	// The keys in this map correspond to fixture names under
	// the "testdata" directory.
	tests := map[string]struct {
		want    *OCIDefaultCredentials
		wantErr string
	}{
		"oci-default-credentials": {
			&OCIDefaultCredentials{
				DiscoverAmbientCredentials: true,
				DockerStyleConfigFiles: []string{
					"/foo/bar/auth.json",
				},
				DefaultDockerCredentialHelper: "osxkeychain",
			},
			``,
		},
		"oci-default-credentials.json": {
			&OCIDefaultCredentials{
				DiscoverAmbientCredentials: true,
				DockerStyleConfigFiles: []string{
					"/foo/bar/auth.json",
				},
				DefaultDockerCredentialHelper: "osxkeychain",
			},
			``,
		},
		"oci-default-credentials-defaults": {
			&OCIDefaultCredentials{
				DiscoverAmbientCredentials:    true,
				DockerStyleConfigFiles:        nil, // represents "use the default search paths"
				DefaultDockerCredentialHelper: "",  // represents no default credential helper at all
			},
			``,
		},
		"oci-default-credentials-no-docker": {
			&OCIDefaultCredentials{
				DiscoverAmbientCredentials: true,
				DockerStyleConfigFiles:     []string{
					// Must be non-nil empty, because nil represents
					// "use the default search paths".
				},
				DefaultDockerCredentialHelper: "", // represents no default credential helper at all
			},
			``,
		},
		"oci-default-credentials-inconsistent": {
			&OCIDefaultCredentials{
				// The following is just a best-effort approximation of the
				// configuration despite the errors, so it's not super important
				// that it stay consistent in future releases but tested just
				// so that if it _does_ change we can review and make sure that
				// the change is reasonable.
				DiscoverAmbientCredentials:    false,
				DockerStyleConfigFiles:        []string{},
				DefaultDockerCredentialHelper: "",
			},
			`disables discovery of ambient credentials, but also sets docker_style_config_files`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fixtureFile := filepath.Join("testdata", name)
			gotConfig, diags := loadConfigFile(fixtureFile)
			if diags.HasErrors() {
				errStr := diags.Err().Error()
				if test.wantErr == "" {
					t.Errorf("unexpected errors: %s", errStr)
				}
				if !strings.Contains(errStr, test.wantErr) {
					t.Errorf("missing expected error\nwant substring: %s\ngot: %s", test.wantErr, errStr)
				}
			} else if test.wantErr != "" {
				t.Errorf("unexpected success\nwant error with substring: %s", test.wantErr)
			}

			var got *OCIDefaultCredentials
			if len(gotConfig.OCIDefaultCredentials) > 0 {
				got = gotConfig.OCIDefaultCredentials[0]
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Error("unexpected result\n" + diff)
			}
		})
	}

	t.Run("oci-default-credentials-duplicate", func(t *testing.T) {
		// This one is different than all of the others because it
		// only gets detected as invalid during the validation step,
		// so that (in the normal case) we can check it only after
		// we've merged all of the separate CLI config files together.
		fixtureFile := filepath.Join("testdata", "oci-default-credentials-duplicate")
		gotConfig, loadDiags := loadConfigFile(fixtureFile)
		if loadDiags.HasErrors() {
			t.Errorf("unexpected errors from loadConfigFile: %s", loadDiags.Err().Error())
		}

		validateDiags := gotConfig.Validate()
		wantErr := `No more than one oci_default_credentials block may be specified`
		if !validateDiags.HasErrors() {
			t.Fatalf("unexpected success\nwant error with substring: %s", wantErr)
		}
		if errStr := validateDiags.Err().Error(); !strings.Contains(errStr, wantErr) {
			t.Errorf("missing expected error\nwant substring: %s\ngot: %s", wantErr, errStr)
		}
	})
}

func TestLoadConfig_ociCredentials(t *testing.T) {
	// The keys in this map correspond to fixture names under
	// the "testdata" directory.
	tests := map[string]struct {
		want    []*OCIRepositoryCredentials
		wantErr string
	}{
		"oci-credentials-basic": {
			[]*OCIRepositoryCredentials{
				{
					RepositoryPrefix: "example.com",
					Username:         "foo",
					Password:         "bar",
				},
			},
			``,
		},
		"oci-credentials-basic.json": {
			[]*OCIRepositoryCredentials{
				{
					RepositoryPrefix: "example.com",
					Username:         "foo",
					Password:         "bar",
				},
			},
			``,
		},
		"oci-credentials-oauth": {
			[]*OCIRepositoryCredentials{
				{
					RepositoryPrefix: "example.com",
					AccessToken:      "foo",
					RefreshToken:     "bar",
				},
			},
			``,
		},
		"oci-credentials-oauth.json": {
			[]*OCIRepositoryCredentials{
				{
					RepositoryPrefix: "example.com",
					AccessToken:      "foo",
					RefreshToken:     "bar",
				},
			},
			``,
		},
		"oci-credentials-credhelper": {
			[]*OCIRepositoryCredentials{
				{
					RepositoryPrefix:       "example.com",
					DockerCredentialHelper: "osxkeychain",
				},
			},
			``,
		},
		"oci-credentials-credhelper.json": {
			[]*OCIRepositoryCredentials{
				{
					RepositoryPrefix:       "example.com",
					DockerCredentialHelper: "osxkeychain",
				},
			},
			``,
		},
		"oci-credentials-multi": {
			[]*OCIRepositoryCredentials{
				{
					RepositoryPrefix: "example.com",
					Username:         "foo",
					Password:         "bar",
				},
				{
					RepositoryPrefix: "example.net",
					Username:         "baz",
					Password:         "beep",
				},
			},
			``,
		},
		"oci-credentials-multi.json": {
			[]*OCIRepositoryCredentials{
				{
					RepositoryPrefix: "example.com",
					Username:         "foo",
					Password:         "bar",
				},
				{
					RepositoryPrefix: "example.net",
					Username:         "baz",
					Password:         "beep",
				},
			},
			``,
		},
		"oci-credentials-empty": {
			nil,
			`must set either username+password, access_token+refresh_token, or docker_credentials_helper`,
		},
		"oci-credentials-mixedstyles": {
			nil,
			`must set only one group out of username+password, access_token+refresh_token, or docker_credentials_helper`,
		},
		"oci-credentials-basic-nopassword": {
			nil,
			`must set both username and password together when using static credentials`,
		},
		"oci-credentials-basic-nousername": {
			nil,
			`must set both username and password together when using static credentials`,
		},
		"oci-credentials-oauth-noaccess": {
			nil,
			`must set both access_token and refresh_token together when using OAuth-style credentials`,
		},
		"oci-credentials-oauth-norefresh": {
			nil,
			`must set both access_token and refresh_token together when using OAuth-style credentials`,
		},
		"oci-credentials-credhelper-badsyntax": {
			nil,
			`specifies the invalid Docker credential helper name "not/valid"`,
		},
		"oci-credentials-credhelper-repopath": {
			nil,
			`cannot set docker_credentials_helper with a repository path`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fixtureFile := filepath.Join("testdata", name)
			gotConfig, diags := loadConfigFile(fixtureFile)
			if diags.HasErrors() {
				errStr := diags.Err().Error()
				if test.wantErr == "" {
					t.Errorf("unexpected errors: %s", errStr)
				}
				if !strings.Contains(errStr, test.wantErr) {
					t.Errorf("missing expected error\nwant substring: %s\ngot: %s", test.wantErr, errStr)
				}
			} else if test.wantErr != "" {
				t.Errorf("unexpected success\nwant error with substring: %s", test.wantErr)
			}

			got := gotConfig.OCIRepositoryCredentials
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Error("unexpected result\n" + diff)
			}
		})
	}

	t.Run("oci-credentials-duplicate", func(t *testing.T) {
		// This one is different than all of the others because it
		// only gets detected as invalid during the validation step,
		// so that (in the normal case) we can check it only after
		// we've merged all of the separate CLI config files together.
		fixtureFile := filepath.Join("testdata", "oci-credentials-duplicate")
		gotConfig, loadDiags := loadConfigFile(fixtureFile)
		if loadDiags.HasErrors() {
			t.Errorf("unexpected errors from loadConfigFile: %s", loadDiags.Err().Error())
		}

		validateDiags := gotConfig.Validate()
		wantErr := `Duplicate oci_credentials block for "example.com"`
		if !validateDiags.HasErrors() {
			t.Fatalf("unexpected success\nwant error with substring: %s", wantErr)
		}
		if errStr := validateDiags.Err().Error(); !strings.Contains(errStr, wantErr) {
			t.Errorf("missing expected error\nwant substring: %s\ngot: %s", wantErr, errStr)
		}
	})
}
