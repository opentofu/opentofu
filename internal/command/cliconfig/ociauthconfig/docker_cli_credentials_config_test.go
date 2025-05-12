// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ociauthconfig

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDockerCLIStyleAuth(t *testing.T) {
	ctx := context.Background()
	configEnv := &fakeConfigDiscoveryEnvironment{
		osName:   "linux",
		homePath: "/home/example",
		files: map[string][]byte{
			"/fake/docker-config-1.json": []byte(`
				{
					"credsStore": "global-helper-1",
					"credHelpers": {
						"example.com": "exampledotcom-helper-1"
					},
					"auths": {
						"example.net": {
							"auth": "` + base64Encode("exampledotnet-user:exampledotnet-password") + `"
						}
					}
				}
			`),
			"/fake/docker-config-2.json": []byte(`
				{
					"credsStore": "global-helper-2",
					"credHelpers": {
						"example.com": "exampledotcom-helper-2"
					},
					"auths": {
						"example.com": {
							"auth": "` + base64Encode("exampledotcom-user:exampledotcom-password") + `"
						},
						"example.com/foo": {
							"auth": "` + base64Encode("exampledotcom-foo-user:exampledotcom-foo-password") + `"
						},
						"example.com/empty": {},
						"example.net/foo": {
							"auth": "` + base64Encode("exampledotnet-foo-user:exampledotnet-foo-password") + `"
						},
						"example.net/foo/bar": {
							"auth": "` + base64Encode("exampledotnet-foo-bar-user:exampledotnet-foo-bar-password") + `"
						},
						"example.net/baz": {
							"auth": "` + base64Encode("exampledotnet-baz-user:exampledotnet-baz-password") + `"
						},
						"empty.example.org": {},
						"null.example.org": null,
						"emptystr.example.org": {"auth":""}
					}
				}
			`),
		},
		t: t,
	}
	configs, err := FixedDockerCLIStyleCredentialsConfigs(ctx, []string{"/fake/docker-config-1.json", "/fake/docker-config-2.json"}, configEnv)
	if err != nil {
		t.Fatal(err)
	}
	allCreds := NewCredentialsConfigs(configs)
	credentialsHelperResults := map[string]map[string]DockerCredentialHelperGetResult{
		"exampledotcom-helper-1": {
			"https://example.com": {
				Username: "from-exampledotcom-helper-1",
				Secret:   "exampledotcom-helper-1-password",
			},
		},
		"global-helper-1": {
			"https://globalcredshelper.example.com": {
				Username: "from-global-helper-1",
				Secret:   "global-helper-1-password",
			},
			"https://empty.example.org": {
				Username: "from-global-helper-1",
				Secret:   "empty.example.org secret",
			},
			"https://null.example.org": {
				Username: "from-global-helper-1",
				Secret:   "null.example.org secret",
			},
			"https://emptystr.example.org": {
				Username: "from-global-helper-1",
				Secret:   "emptystr.example.org secret",
			},
		},
	}

	tests := []struct {
		registryDomain, repositoryPath string
		wantSpecificity                CredentialsSpecificity
		wantCreds                      *Credentials
	}{
		{
			"unconfigured.example.com", "doot",
			GlobalCredentialsSpecificity, // the global credentials helper matches...
			nil,                          // ...but it doesn't return any credentials for this domain
		},
		{
			"globalcredshelper.example.com", "doot",
			GlobalCredentialsSpecificity,
			&Credentials{
				username: "from-global-helper-1",
				password: "global-helper-1-password",
			},
		},
		{
			"example.com", "not-explicitly-configured",
			// This domain has multiple domain-level settings available, so
			// the "declared first" rule wins, causing us to use the
			// exampledotcom-helper-1 credentials helper from the first
			// config file, and ignore the "auths" entry in the second file.
			DomainCredentialsSpecificity,
			&Credentials{
				username: "from-exampledotcom-helper-1",
				password: "exampledotcom-helper-1-password",
			},
		},
		{
			"example.com", "foo",
			// There is a path-based override for this prefix and so
			// that wins over the example.com credentials helper due to
			// having higher specificity.
			RepositoryCredentialsSpecificity(1),
			&Credentials{
				username: "exampledotcom-foo-user",
				password: "exampledotcom-foo-password",
			},
		},
		{
			"example.com", "empty",
			// The object for "example.com/empty" has no "auth" property
			// inside it and so we ignore it completely and use the
			// "example.com" domain-level entry instead.
			DomainCredentialsSpecificity,
			&Credentials{
				username: "from-exampledotcom-helper-1",
				password: "exampledotcom-helper-1-password",
			},
		},
		{
			"example.net", "not-explicitly-configured",
			DomainCredentialsSpecificity,
			&Credentials{
				username: "exampledotnet-user",
				password: "exampledotnet-password",
			},
		},
		{
			"example.net", "foo",
			// There's a path-based override for this prefix.
			RepositoryCredentialsSpecificity(1),
			&Credentials{
				username: "exampledotnet-foo-user",
				password: "exampledotnet-foo-password",
			},
		},
		{
			"example.net", "foo/doot",
			// There's a path-based override for foo which matches, but not foo/doot specifically.
			RepositoryCredentialsSpecificity(1),
			&Credentials{
				username: "exampledotnet-foo-user",
				password: "exampledotnet-foo-password",
			},
		},
		{
			"example.net", "foo/bar",
			// There's a path-based override for this prefix.
			RepositoryCredentialsSpecificity(2),
			&Credentials{
				username: "exampledotnet-foo-bar-user",
				password: "exampledotnet-foo-bar-password",
			},
		},
		{
			"example.net", "baz",
			// There's a path-based override for this prefix.
			RepositoryCredentialsSpecificity(1),
			&Credentials{
				username: "exampledotnet-baz-user",
				password: "exampledotnet-baz-password",
			},
		},
		{
			"empty.example.org", "blah",
			// The JSON config for this domain has an empty object, so we
			// should ignore it completely and thus select the global
			// credentials helper instead.
			GlobalCredentialsSpecificity,
			&Credentials{
				username: "from-global-helper-1",
				password: "empty.example.org secret",
			},
		},
		{
			"null.example.org", "blah",
			// The JSON config for this domain has a null value, so we
			// should ignore it completely and thus select the global
			// credentials helper instead.
			GlobalCredentialsSpecificity,
			&Credentials{
				username: "from-global-helper-1",
				password: "null.example.org secret",
			},
		},
		{
			"emptystr.example.org", "blah",
			// The JSON config for this domain has an empty "auth" string, so we
			// should ignore it completely and thus select the global
			// credentials helper instead.
			GlobalCredentialsSpecificity,
			&Credentials{
				username: "from-global-helper-1",
				password: "emptystr.example.org secret",
			},
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s/%s", test.registryDomain, test.repositoryPath), func(t *testing.T) {
			source, err := allCreds.CredentialsSourceForRepository(ctx, test.registryDomain, test.repositoryPath)
			if IsCredentialsNotFoundError(err) {
				if test.wantCreds != nil {
					t.Fatalf("wanted credentials but got error: %s", err)
				}
				return // Success: we didn't want any credentials for this one
			} else if err != nil {
				t.Fatal(err)
			}
			gotSpecificity := NoCredentialsSpecificity
			if source != nil {
				gotSpecificity = source.CredentialsSpecificity()
			}
			if gotSpecificity != test.wantSpecificity {
				t.Errorf("wrong specificity\ngot:  %#v\nwant: %#v", gotSpecificity, test.wantSpecificity)
			}
			if source == nil {
				t.Fatal("no error, but also no source")
			}

			lookupEnv := &fakeCredentialsLookupEnvironment{
				credentialsHelperResults: credentialsHelperResults,
				t:                        t,
			}
			creds, err := source.Credentials(ctx, lookupEnv)
			if IsCredentialsNotFoundError(err) {
				if test.wantCreds != nil {
					t.Fatalf("wanted credentials but got error: %s", err)
				}
				return // Success: we didn't want any credentials for this one
			} else if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.wantCreds, &creds, cmpOptions); diff != "" {
				t.Error("wrong credentials\n" + diff)
			}
		})
	}
}

func TestDockerCLIStyleAuthFileSearchLocations(t *testing.T) {
	// This directly tests the unexported dockerCLIStyleAuthFileSearchLocations helper, which
	// does its work based entirely on the metadata in the environment object without actually
	// accessing the filesystem.
	tests := map[string]struct {
		env       fakeConfigDiscoveryEnvironment
		wantPaths []string
	}{
		"linux with empty environment": {
			fakeConfigDiscoveryEnvironment{
				osName:   "linux",
				homePath: "/home/example",
			},
			[]string{
				"/home/example/.config/containers/auth.json",
				"/home/example/.docker/config.json",
				"/home/example/.dockercfg",
			},
		},
		"linux with XDG basedir enviroment variables": {
			fakeConfigDiscoveryEnvironment{
				osName:   "linux",
				homePath: "/home/example",
				envVars: map[string]string{
					"XDG_RUNTIME_DIR": "/var/run/12",
					"XDG_CONFIG_HOME": "/home/example/.contrarian-config",
				},
			},
			[]string{
				"/var/run/12/containers/auth.json",
				"/home/example/.contrarian-config/containers/auth.json",
				"/home/example/.docker/config.json",
				"/home/example/.dockercfg",
			},
		},
		"windows with empty environment": {
			fakeConfigDiscoveryEnvironment{
				osName:   "windows",
				homePath: "c:/Users/Example",
			},
			[]string{
				"c:/Users/Example/.config/containers/auth.json",
				"c:/Users/Example/.config/containers/auth.json", // the documented search rules cause this to be duplicated when XDG_CONFIG_HOME is not set
				"c:/Users/Example/.docker/config.json",
				"c:/Users/Example/.dockercfg",
			},
		},
		"windows with XDG basedir environment variables": {
			// This particular test is weird because XDG doesn't really belong on Windows, but
			// other container software has minimal support for it and so we follow their lead.
			fakeConfigDiscoveryEnvironment{
				osName:   "windows",
				homePath: "c:/Users/Example",
				envVars: map[string]string{
					"XDG_RUNTIME_DIR": "c:/Temp/whatever", // ignored when not on Linux
					"XDG_CONFIG_HOME": "c:/Users/Example/xdg-config",
				},
			},
			[]string{
				"c:/Users/Example/.config/containers/auth.json",
				"c:/Users/Example/xdg-config/containers/auth.json",
				"c:/Users/Example/.docker/config.json",
				"c:/Users/Example/.dockercfg",
			},
		},
		"darwin with empty environment": {
			fakeConfigDiscoveryEnvironment{
				osName:   "darwin",
				homePath: "/Users/example",
			},
			[]string{
				"/Users/example/.config/containers/auth.json",
				"/Users/example/.config/containers/auth.json", // the documented search rules cause this to be duplicated when XDG_CONFIG_HOME is not set
				"/Users/example/.docker/config.json",
				"/Users/example/.dockercfg",
			},
		},
		"darwin with XDG basedir environment variables": {
			fakeConfigDiscoveryEnvironment{
				osName:   "darwin",
				homePath: "/Users/example",
				envVars: map[string]string{
					"XDG_RUNTIME_DIR": "/System/temp/whatever", // ignored when not on Linux
					"XDG_CONFIG_HOME": "/Users/example/xdg-config",
				},
			},
			[]string{
				"/Users/example/.config/containers/auth.json",
				"/Users/example/xdg-config/containers/auth.json",
				"/Users/example/.docker/config.json",
				"/Users/example/.dockercfg",
			},
		},
		"other OS with empty environment": {
			fakeConfigDiscoveryEnvironment{
				osName:   "anythingelse",
				homePath: "/home/example",
			},
			[]string{
				"/home/example/.config/containers/auth.json",
				"/home/example/.docker/config.json",
				"/home/example/.dockercfg",
			},
		},
		"other OS with XDG basedir environment variables": {
			fakeConfigDiscoveryEnvironment{
				osName:   "anythingelse",
				homePath: "/home/example",
				envVars: map[string]string{
					"XDG_RUNTIME_DIR": "/var/run/12", // ignored when not on Linux
					"XDG_CONFIG_HOME": "/home/example/.contrarian-config",
				},
			},
			[]string{
				// XDG_RUNTIME_DIR is consulted only on Linux
				"/home/example/.contrarian-config/containers/auth.json",
				"/home/example/.docker/config.json",
				"/home/example/.dockercfg",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			gotPaths := dockerCLIStyleAuthFileSearchLocations(&test.env)
			for i := range gotPaths {
				gotPaths[i] = normalizeFilePath(gotPaths[i])
			}
			if diff := cmp.Diff(test.wantPaths, gotPaths); diff != "" {
				t.Error("wrong result\n" + diff)
			}
		})
	}
}

func TestContainersAuthPropertyNameMatch(t *testing.T) {
	tests := []struct {
		authsPropertyName                        string
		matchRegistryDomain, matchRepositoryPath string
		wantSpecificity                          CredentialsSpecificity
	}{
		{
			"example.net",
			"example.net", "foo",
			DomainCredentialsSpecificity,
		},
		{
			"example.net/foo",
			"example.net", "foo",
			RepositoryCredentialsSpecificity(1),
		},
		{
			"example.net/foo",
			"example.net", "foo/bar",
			RepositoryCredentialsSpecificity(1), // prefix match
		},
		{
			"example.net/foo/bar",
			"example.net", "foo",
			NoCredentialsSpecificity,
		},
		{
			"example.net/foo/bar",
			"example.net", "foo/bar",
			RepositoryCredentialsSpecificity(2),
		},
		{
			"example.net/foo/bar",
			"example.net", "foo/bar/baz",
			RepositoryCredentialsSpecificity(2), // prefix match
		},
		{
			"example.net/foo/not-bar",
			"example.net", "foo/bar",
			NoCredentialsSpecificity,
		},
		{
			"example.net/not-foo",
			"example.net", "foo/bar",
			NoCredentialsSpecificity,
		},
	}

	for _, test := range tests {
		testName := fmt.Sprintf("%s/%s matching against %s", test.matchRegistryDomain, test.matchRepositoryPath, test.authsPropertyName)
		t.Run(testName, func(t *testing.T) {
			t.Log(testName) // more readable without the transforming t.Run does to the name

			gotSpecificity := ContainersAuthPropertyNameMatch(
				test.authsPropertyName,
				test.matchRegistryDomain, test.matchRepositoryPath,
			)
			if gotSpecificity != test.wantSpecificity {
				t.Errorf("wrong specificity:\ngot:  %#v\nwant: %#v", gotSpecificity, test.wantSpecificity)
			}
		})
	}
}
