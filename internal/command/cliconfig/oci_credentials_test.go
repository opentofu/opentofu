// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/opentofu/opentofu/internal/command/cliconfig/ociauthconfig"
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

func TestConfigOCICredentialsPolicy(t *testing.T) {
	// This test exercises various different combinations of OCI credentials
	// policy configuration, verifying that the correct set of credentials gets
	// selected in each case as a limited-scope integration test.
	//
	// This is the most comprehensive and complicated test of the OCI credentials
	// policy functionality, and there are various more-focused unit tests both
	// elsewhere in this package and in package ociauthconfig, so if you can test
	// whatever you are trying to test in a more direct way using one of those
	// other tests that would probably give us more specific feedback if
	// something gets regressed in future.
	//
	// This test also intentionally focuses only on valid configuration cases,
	// since the Config.OCICredentialsPolicy method is documented as allowed only
	// after having successfully loaded and validated a CLI configuration. To
	// test the handling of invalid configuration input, add cases to
	// TestLoadConfig_ociDefaultCredentials and TestLoadConfig_ociCredentials instead.

	type Subtest struct {
		wantSpecificity ociauthconfig.CredentialsSpecificity
		wantCredentials *ociauthconfig.Credentials
	}
	fixturesDir := filepath.Join("testdata", "oci-credentials-policy")

	// The keys of the following map correspond to directories under
	// testdata/oci-credentials-policy. Each directory should include
	// zero or more files named with the ".tfrc" or ".tfrc.json" suffix,
	// and can optionally include the following subdirectories:
	// - home: represents a fake home directory to use when performing
	//   discovery of ambient credentials. This directory will be provided
	//   to the discovery code regardless of whether it exists, with the
	//   assumption that any test fixture that needs it will include it.
	// - xdgconfig: if present, the absolute path to this directory is
	//   provided to the ambient config discovery code as the
	//   XDG_CONFIG_HOME environment variable.
	// - xdgrun: if present, the absolute path to this directory is
	//   provided to the ambient config discovery code as the
	//   XDG_RUNTIME_DIR environment variable.
	// Each directory name must end with a dash followed by a GOOS-style
	// operating system name, which will be used to influence the
	// platform-specific rules in the ambient config discovery logic.
	tests := map[string]struct {
		// wantConfigLocations is the set of expected config location names as reported
		// by the CredentialsConfigLocationForUI method of each CredentialsConfig object.
		//
		// This is a baseline check to make sure we even discovered what we expected to
		// discover, to give more explicit feedback about inconsistencies than we'd get
		// just from the subtests, which only _indirectly_ test that the expected config
		// locations are present.
		wantConfigLocations []string

		// The keys of this map are OCI repository addresses like "domainname/repository"
		// which should be looked up against the credentials policy represented by the
		// configuration of the parent test, and should produce the given results.
		subtests map[string]Subtest
	}{
		"empty-linux": {
			wantConfigLocations: nil,
			subtests: map[string]Subtest{
				"example.com": {
					wantCredentials: nil,
				},
				"example.com/foo/bar": {
					wantCredentials: nil,
				},
			},
		},
		"mixed-darwin": {
			wantConfigLocations: []string{
				`explicit oci_credentials "example.com" block`,
				`explicit oci_credentials "example.com/foo" block`,
				`oci_default_credentials block`,
				`home/.config/containers/auth.json`,
			},
			subtests: map[string]Subtest{
				"example.org": {
					// We have no blocks for this domain, so the global credential helper "wins".
					// If this fails trying to use a credhelper called "superseded-by-explicit-config"
					// then we incorrectly preferred the ambient config's setting for this.
					wantSpecificity: ociauthconfig.GlobalCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("from-cred-helper", "for https://example.org")),
				},
				"example.com": {
					// This domain has a conflicting entry in the ambient Docker-style config,
					// but the explicit configuration should "win".
					wantSpecificity: ociauthconfig.DomainCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("example.com user", "example.com password")),
				},
				"example.com/foo": {
					wantSpecificity: ociauthconfig.RepositoryCredentialsSpecificity(1),
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("example.com/foo user", "example.com/foo password")),
				},
				"example.com/bar": {
					// These credentials come from the base64-encoded "auth" string in the
					// ambient Docker-style config file.
					wantSpecificity: ociauthconfig.RepositoryCredentialsSpecificity(1),
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("ambient-example.com-user", "ambient-password")),
				},
				"example.com/not-foo": {
					wantSpecificity: ociauthconfig.DomainCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("example.com user", "example.com password")),
				},
				"example.com/not-bar": {
					wantSpecificity: ociauthconfig.DomainCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("example.com user", "example.com password")),
				},
			},
		},
		"explicit-specificity-linux": {
			wantConfigLocations: []string{
				`explicit oci_credentials "example.com" block`,
				`explicit oci_credentials "example.com/foo" block`,
				`explicit oci_credentials "example.com/foo/bar" block`,
				`explicit oci_credentials "example.net" block`,
				`explicit oci_credentials "example.net/foo" block`,
			},
			subtests: map[string]Subtest{
				"example.org": {
					wantCredentials: nil, // no credentials blocks for this domain at all
				},
				"example.com": {
					wantSpecificity: ociauthconfig.DomainCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("example.com user", "example.com password")),
				},
				"example.com/foo": {
					wantSpecificity: ociauthconfig.RepositoryCredentialsSpecificity(1),
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("example.com/foo user", "example.com/foo password")),
				},
				"example.com/foo/bar": {
					wantSpecificity: ociauthconfig.RepositoryCredentialsSpecificity(2),
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("example.com/foo/bar user", "example.com/foo/bar password")),
				},
				"example.com/foo/not-bar": {
					wantSpecificity: ociauthconfig.RepositoryCredentialsSpecificity(1),
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("example.com/foo user", "example.com/foo password")),
				},
				"example.com/not-foo/not-bar": {
					wantSpecificity: ociauthconfig.DomainCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("example.com user", "example.com password")),
				},
				"example.net": {
					wantSpecificity: ociauthconfig.DomainCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("from-cred-helper", "for https://example.net")),
				},
				"example.net/foo": {
					wantSpecificity: ociauthconfig.RepositoryCredentialsSpecificity(1),
					wantCredentials: ptrTo(ociauthconfig.NewOAuthCredentials("example.net/foo access", "example.net/foo refresh")),
				},
				"example.net/not-foo": {
					wantSpecificity: ociauthconfig.DomainCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("from-cred-helper", "for https://example.net")),
				},
			},
		},
		"explicit-global-credhelper-linux": {
			wantConfigLocations: []string{
				"oci_default_credentials block",
			},
			subtests: map[string]Subtest{
				"example.com/foo/bar": {
					wantSpecificity: ociauthconfig.GlobalCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("from-cred-helper", "for https://example.com")),
				},
				"example.net": {
					wantSpecificity: ociauthconfig.GlobalCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("from-cred-helper", "for https://example.net")),
				},
			},
		},

		// These ambient-global-credhelper- tests are the main way we're exercising
		// the code that deals with finding Docker-style configuration files, since
		// a global credentials helper gives us a straightforward signal about whether
		// we found the file or not. We don't need to duplicate all of these different
		// search cases for other kinds of credentials source because we use the
		// same discovery code regardless of what kind of configuration we might
		// find in the files we find.
		//
		// Note that the logic for deciding which paths to search for Docker-like
		// configuration files has its own unit test in package ociauthconfig, and so
		// we don't necessarily need to cover all of the combinations of different file
		// paths here too.
		"ambient-global-credhelper-xdgconfig-linux": {
			wantConfigLocations: []string{
				filepath.Join("xdgconfig", "containers", "auth.json"),
			},
			subtests: map[string]Subtest{
				"example.com/foo/bar": {
					wantSpecificity: ociauthconfig.GlobalCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("from-cred-helper", "for https://example.com")),
				},
				"example.net": {
					wantSpecificity: ociauthconfig.GlobalCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("from-cred-helper", "for https://example.net")),
				},
			},
		},
		"ambient-global-credhelper-xdgrun-linux": {
			wantConfigLocations: []string{
				filepath.Join("xdgrun", "containers", "auth.json"),
			},
			subtests: map[string]Subtest{
				"example.net": {
					wantSpecificity: ociauthconfig.GlobalCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("from-cred-helper", "for https://example.net")),
				},
			},
		},
		"ambient-global-credhelper-xdgdefault-linux": {
			wantConfigLocations: []string{
				filepath.Join("home", ".config", "containers", "auth.json"),
			},
			subtests: map[string]Subtest{
				"example.net": {
					wantSpecificity: ociauthconfig.GlobalCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("from-cred-helper", "for https://example.net")),
				},
			},
		},
		"ambient-global-credhelper-docker-linux": {
			wantConfigLocations: []string{
				filepath.Join("home", ".docker", "config.json"),
			},
			subtests: map[string]Subtest{
				"example.net": {
					wantSpecificity: ociauthconfig.GlobalCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("from-cred-helper", "for https://example.net")),
				},
			},
		},
		"ambient-global-credhelper-dockerlegacy-linux": {
			wantConfigLocations: []string{
				filepath.Join("home", ".dockercfg"),
			},
			subtests: map[string]Subtest{
				"example.net": {
					wantSpecificity: ociauthconfig.GlobalCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("from-cred-helper", "for https://example.net")),
				},
			},
		},
		"ambient-global-credhelper-explicitpath-linux": {
			wantConfigLocations: []string{
				"explicitly-named.json",
			},
			subtests: map[string]Subtest{
				"example.net": {
					// This should select the "fake" credential helper from the first
					// location above, and thus ignore all of the other credential
					// helper names configured in the other configuration files.
					// If this fails trying to use a different credentials helper
					// then that suggests that we're not respecting file preference order.
					wantSpecificity: ociauthconfig.GlobalCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("from-cred-helper", "for https://example.net")),
				},
			},
		},
		"ambient-global-credhelper-various-linux": {
			wantConfigLocations: []string{
				filepath.Join("xdgrun", "containers", "auth.json"),
				// home/.config/containers/auth.json is ignored on Linux whenever XDG_CONFIG_HOME is set
				filepath.Join("xdgconfig", "containers", "auth.json"),
				filepath.Join("home", ".docker", "config.json"),
				filepath.Join("home", ".dockercfg"),
			},
			subtests: map[string]Subtest{
				"example.net": {
					// This should select the "fake" credential helper from the first
					// location above, and thus ignore all of the other credential
					// helper names configured in the other configuration files.
					// If this fails trying to use a different credentials helper
					// then that suggests that we're not respecting file preference order.
					wantSpecificity: ociauthconfig.GlobalCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("from-cred-helper", "for https://example.net")),
				},
			},
		},
		"ambient-global-credhelper-various-windows": {
			wantConfigLocations: []string{
				// the xdgrun/containers/auth.json file is ignored on Windows
				filepath.Join("home", ".config", "containers", "auth.json"),
				filepath.Join("xdgconfig", "containers", "auth.json"),
				filepath.Join("home", ".docker", "config.json"),
				filepath.Join("home", ".dockercfg"),
			},
			subtests: map[string]Subtest{
				"example.net": {
					// This should select the "fake" credential helper from the first
					// location above, and thus ignore all of the other credential
					// helper names configured in the other configuration files.
					// If this fails trying to use a different credentials helper
					// then that suggests that we're not respecting file preference order.
					wantSpecificity: ociauthconfig.GlobalCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("from-cred-helper", "for https://example.net")),
				},
			},
		},
		"ambient-global-credhelper-various-darwin": {
			wantConfigLocations: []string{
				// the xdgrun/containers/auth.json file is ignored on macOS
				filepath.Join("home", ".config", "containers", "auth.json"),
				filepath.Join("xdgconfig", "containers", "auth.json"),
				filepath.Join("home", ".docker", "config.json"),
				filepath.Join("home", ".dockercfg"),
			},
			subtests: map[string]Subtest{
				"example.net": {
					// This should select the "fake" credential helper from the first
					// location above, and thus ignore all of the other credential
					// helper names configured in the other configuration files.
					// If this fails trying to use a different credentials helper
					// then that suggests that we're not respecting file preference order.
					wantSpecificity: ociauthconfig.GlobalCredentialsSpecificity,
					wantCredentials: ptrTo(ociauthconfig.NewBasicAuthCredentials("from-cred-helper", "for https://example.net")),
				},
			},
		},
		"ambient-totally-disabled-linux": {
			wantConfigLocations: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var osName string
			if lastDashIdx := strings.LastIndexByte(name, '-'); lastDashIdx == -1 {
				t.Fatalf("test name does not include -osname suffix")
			} else {
				osName = name[lastDashIdx+1:]
			}

			configDir := filepath.Join(fixturesDir, name)
			absConfigDir, err := filepath.Abs(configDir)
			if err != nil {
				t.Fatalf("can't get absolute path for %s", configDir)
			}
			cfg, diags := loadConfigDir(configDir)
			if diags.HasErrors() {
				t.Fatalf("errors loading config: %s", diags.Err().Error())
			}
			diags = cfg.Validate()
			if diags.HasErrors() {
				t.Fatalf("invalid config:\n%s", diags.Err().Error())
			}

			baseDir, err := filepath.Abs(configDir)
			if err != nil {
				t.Fatalf("cannot make path %q absolute: %s", configDir, err)
			}
			discoEnv := &fakeOCIConfigDiscoveryEnvironment{
				osName:        osName,
				homePath:      filepath.Join(baseDir, "home"),
				xdgConfigHome: filepath.Join(baseDir, "xdgconfig"),
				xdgRuntimeDir: filepath.Join(baseDir, "xdgrun"),
			}
			if _, err = os.Stat(discoEnv.xdgConfigHome); os.IsNotExist(err) {
				discoEnv.xdgConfigHome = ""
			}
			if _, err = os.Stat(discoEnv.xdgRuntimeDir); os.IsNotExist(err) {
				discoEnv.xdgRuntimeDir = ""
			}
			policy, err := cfg.ociCredentialsPolicy(t.Context(), discoEnv)
			if err != nil {
				t.Fatalf("error building credentials policy: %s", err)
			}
			var gotConfigLocations []string
			for _, credCfg := range policy.AllConfigs() {
				loc := credCfg.CredentialsConfigLocationForUI()
				// Sometimes the locations are absolute file paths, so we'll make a best effort
				// to re-relativize them so that our test table doesn't need to deal with variations
				// in base directory between different dev environments. If this doesn't work then
				// we'll assume it's a non-filepath-based location string, which is fine.
				if relLoc, err := filepath.Rel(absConfigDir, loc); err == nil {
					loc = relLoc
				}
				gotConfigLocations = append(gotConfigLocations, loc)
			}
			if diff := cmp.Diff(test.wantConfigLocations, gotConfigLocations); diff != "" {
				t.Error("wrong configuration locations\n" + diff)
			}

			for subname, subtest := range test.subtests {
				t.Run(subname, func(t *testing.T) {
					registryDomain, repositoryPath, err := ociauthconfig.ParseRepositoryAddressPrefix(subname)
					if err != nil {
						t.Fatalf("subtest has invalid repository address: %s", err)
					}

					credSrc, err := policy.CredentialsSourceForRepository(t.Context(), registryDomain, repositoryPath)
					if ociauthconfig.IsCredentialsNotFoundError(err) {
						t.Logf("no credentials found: %s", err.Error())
						if subtest.wantCredentials != nil {
							t.Errorf("no credentials returned, but want %s", spew.Sdump(subtest.wantCredentials))
						}
						return // successfully found no credentials, as expected
					} else if err != nil {
						// This test is only for valid cases, so any other error is a test failure.
						t.Fatalf("failed to get credentials source: %s", err)
					}
					if got, want := credSrc.CredentialsSpecificity(), subtest.wantSpecificity; got != want {
						t.Errorf("wrong specificity\ngot:  %#v\nwant: %#v", got, want)
					}
					t.Logf("found credentials for %s/%s at specificity %#v", registryDomain, repositoryPath, credSrc.CredentialsSpecificity())

					lookupEnv := &fakeOCICredLookupEnvironment{
						regDomain: registryDomain,
					}
					creds, err := credSrc.Credentials(t.Context(), lookupEnv)
					if ociauthconfig.IsCredentialsNotFoundError(err) {
						t.Logf("no credentials found: %s", err.Error())
					} else if err != nil && !ociauthconfig.IsCredentialsNotFoundError(err) {
						// This test is only for valid cases, so any other error is a test failure.
						t.Fatalf("failed to get credentials source: %s", err)
					}
					if diff := cmp.Diff(subtest.wantCredentials, &creds, cmpopts.EquateComparable(ociauthconfig.Credentials{})); diff != "" {
						t.Error("wrong credentials\n" + diff)
					}
				})
			}
		})
	}
}

type fakeOCIConfigDiscoveryEnvironment struct {
	osName        string
	homePath      string
	xdgConfigHome string
	xdgRuntimeDir string
}

var _ ociauthconfig.ConfigDiscoveryEnvironment = (*fakeOCIConfigDiscoveryEnvironment)(nil)

// EnvironmentVariableVal implements ociauthconfig.ConfigDiscoveryEnvironment.
func (e *fakeOCIConfigDiscoveryEnvironment) EnvironmentVariableVal(name string) string {
	switch name {
	case "XDG_CONFIG_HOME":
		return e.xdgConfigHome
	case "XDG_RUNTIME_DIR":
		return e.xdgRuntimeDir
	default:
		return ""
	}
}

// OperatingSystemName implements ociauthconfig.ConfigDiscoveryEnvironment.
func (e *fakeOCIConfigDiscoveryEnvironment) OperatingSystemName() string {
	return e.osName
}

// ReadFile implements ociauthconfig.ConfigDiscoveryEnvironment.
func (e *fakeOCIConfigDiscoveryEnvironment) ReadFile(_ context.Context, path string) ([]byte, error) {
	// We don't fake out the actual file reads, because we assume that the
	// tests will ensure that all of the paths reported by other methods
	// are absolute paths pointing into a test fixture directory.
	return os.ReadFile(path)
}

// UserHomeDirPath implements ociauthconfig.ConfigDiscoveryEnvironment.
func (e *fakeOCIConfigDiscoveryEnvironment) UserHomeDirPath() string {
	return e.homePath
}

type fakeOCICredLookupEnvironment struct {
	regDomain string
}

// QueryDockerCredentialHelper implements ociauthconfig.CredentialsLookupEnvironment.
func (f *fakeOCICredLookupEnvironment) QueryDockerCredentialHelper(ctx context.Context, helperName string, serverURL string) (ociauthconfig.DockerCredentialHelperGetResult, error) {
	if helperName != "fake" {
		return ociauthconfig.DockerCredentialHelperGetResult{}, ociauthconfig.NewCredentialsNotFoundError(fmt.Errorf("only the 'fake' credential helper is available in this testing environment"))
	}
	if got, want := serverURL, "https://"+f.regDomain; got != want {
		return ociauthconfig.DockerCredentialHelperGetResult{}, ociauthconfig.NewCredentialsNotFoundError(fmt.Errorf("fake credentials helper only has credentials for %s", want))
	}
	return ociauthconfig.DockerCredentialHelperGetResult{
		ServerURL: serverURL,
		Username:  "from-cred-helper",
		Secret:    "for " + serverURL,
	}, nil
}

// ptrTo is a helper to compensate for the fact that Go doesn't allow
// using the '&' operator unless the operand is directly addressable.
//
// Instead then, this function returns a pointer to a copy of the given
// value.
func ptrTo[T any](v T) *T {
	return &v
}
