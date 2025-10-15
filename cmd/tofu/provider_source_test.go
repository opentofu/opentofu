// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/cliconfig"
	"github.com/opentofu/opentofu/internal/command/cliconfig/ociauthconfig"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/svchost/disco"
)

func TestProviderSource(t *testing.T) {
	tests := []struct {
		name             string
		setupFunc        func(t *testing.T) (string, string) // returns (originalDir, overrideDir)
		expectedProvider string
	}{
		{
			name: "without overrideWd - should use terraform.d/plugins in original working directory",
			setupFunc: func(t *testing.T) (string, string) {
				// Create a temporary directory to conduct the test in
				tempDir := t.TempDir()

				// Create terraform.d/plugins in the testing directory
				pluginsDir := filepath.Join(tempDir, "terraform.d", "plugins")
				err := os.MkdirAll(pluginsDir, 0755)
				if err != nil {
					t.Fatalf("Failed to create plugins directory: %v", err)
				}

				// Create a mock provider in the plugins directory
				providerDir := filepath.Join(pluginsDir, "registry.opentofu.org", "hashicorp", "test-provider", "1.0.0", "linux_amd64")
				err = os.MkdirAll(providerDir, 0755)
				if err != nil {
					t.Fatalf("Failed to create provider directory: %v", err)
				}

				// Create a mock provider binary
				providerBinary := filepath.Join(providerDir, "terraform-provider-test-provider")
				err = os.WriteFile(providerBinary, []byte("mock provider binary"), 0755)
				if err != nil {
					t.Fatalf("Failed to create mock provider binary: %v", err)
				}

				return tempDir, ""
			},
			expectedProvider: "hashicorp/test-provider",
		},
		{
			name: "with overrideWd - should still use terraform.d/plugins in original working directory",
			setupFunc: func(t *testing.T) (string, string) {
				// Create a temporary directory to conduct the test in
				tempDir := t.TempDir()

				// Create terraform.d/plugins in the testing directory
				pluginsDir := filepath.Join(tempDir, "terraform.d", "plugins")
				err := os.MkdirAll(pluginsDir, 0755)
				if err != nil {
					t.Fatalf("Failed to create plugins directory: %v", err)
				}

				// Create a mock provider in the plugins directory
				providerDir := filepath.Join(pluginsDir, "registry.opentofu.org", "hashicorp", "test-provider", "1.0.0", "linux_amd64")
				err = os.MkdirAll(providerDir, 0755)
				if err != nil {
					t.Fatalf("Failed to create provider directory: %v", err)
				}

				// Create a mock provider binary
				providerBinary := filepath.Join(providerDir, "terraform-provider-test-provider")
				err = os.WriteFile(providerBinary, []byte("mock provider binary"), 0755)
				if err != nil {
					t.Fatalf("Failed to create mock provider binary: %v", err)
				}

				// Create a temporary directory for the override working directory
				overrideDir := filepath.Join(tempDir, "override")
				err = os.MkdirAll(overrideDir, 0755)
				if err != nil {
					t.Fatalf("Failed to create override directory: %v", err)
				}

				return tempDir, overrideDir
			},
			expectedProvider: "hashicorp/test-provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment
			originalWorkingDir, overrideWd := tt.setupFunc(t)

			t.Chdir(originalWorkingDir)

			// If we have an override directory, change to it (simulating -chdir behavior)
			if overrideWd != "" {
				t.Chdir(overrideWd)
			}

			// Create a mock disco service
			services := disco.New()

			// Create a mock OCI credentials policy builder
			ociCredsPolicy := func(ctx context.Context) (ociauthconfig.CredentialsConfigs, error) {
				return ociauthconfig.CredentialsConfigs{}, nil
			}

			// Call the function under test
			source, diags := providerSource(
				context.Background(),
				[]*cliconfig.ProviderInstallation{},
				&cliconfig.RegistryProtocolsConfig{
					RetryCount:        1,
					RetryCountSet:     true,
					RequestTimeout:    10 * time.Second,
					RequestTimeoutSet: true,
				},
				services,
				ociCredsPolicy,
				originalWorkingDir,
			)

			// Verify no diagnostics were returned
			if len(diags) > 0 {
				t.Fatalf("Expected no diagnostics, got: %v", diags)
			}

			// Verify the source is not nil
			if source == nil {
				t.Fatal("Expected source to be non-nil")
			}

			// Try to get available versions for a test provider
			// This will help verify that the local directories are properly configured
			provider := addrs.MustParseProviderSourceString(tt.expectedProvider)
			versions, _, err := source.AvailableVersions(context.Background(), provider)
			if err != nil {
				// If available provider versions could not be determined, something went wrong with mock provider setup
				t.Fatalf("AvailableVersions failed (expected for test): %v", err)
			}

			t.Logf("Source created successfully for test case: %s", tt.name)
			t.Logf("Provider: %v", provider)
			if versions != nil {
				t.Logf("Available versions: %v", versions)
			}
		})
	}
}

// TestInitProviderSourceForCLIConfigLocationWithRetries tests how the retries are handled and passed over to the
// [getproviders.PackageLocation].
// Normally, this should have been an e2e test, but there is no easy way to mock a `direct` registry
// to be able to test that the retries actually work properly.
//
// This test contains no tests for [cliconfig.ProviderInstallationNetworkMirror] on purpose since
// there is no way to create [getproviders.HTTPMirrorSource]
// with an on-the-fly generated TLS certificate since it's required by the
// [getproviders.HTTPMirrorSource] inner working.
// Instead, the retries functionality for `network_mirror` is tested into an e2e called
// "TestProviderNetworkMirrorRetries" since that way we were able to provide the TLS certificate
// into the child process instead.
func TestInitProviderSourceForCLIConfigLocationWithRetries(t *testing.T) {
	registryHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/providers/v1/test/exists/0.0.0/download/foo_os/bar_arch":
			_, _ = w.Write([]byte(`{"os":"foo_os","arch":"bar_arch","download_url":"/providers/v1/test/exists_0.0.0.zip","shasum":"4cbc33c22abdebe3a3679666d4052ec95c40bd8904a9458f90cf934363a14cc7"}`))
		case "/providers/v1/test/exists_0.0.0.zip":
			w.WriteHeader(http.StatusInternalServerError)
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	cases := map[string]struct {
		methodType     cliconfig.ProviderInstallationLocation
		tofurcRetries  cliconfig.ProviderInstallationMethodRetries
		envVars        map[string]string
		expectedErrMsg string
	}{
		"no tofurc.direct.download_retry_count configured, no TF_PROVIDER_DOWNLOAD_RETRY, default TF_PROVIDER_DOWNLOAD_RETRY used": {
			methodType: cliconfig.ProviderInstallationDirect,
			tofurcRetries: func() (int, bool) {
				return 0, false
			},
			envVars:        nil,
			expectedErrMsg: "/providers/v1/test/exists_0.0.0.zip giving up after 3 attempt(s)",
		},
		"no tofurc.direct.download_retry_count, TF_PROVIDER_DOWNLOAD_RETRY defined, TF_PROVIDER_DOWNLOAD_RETRY used": {
			methodType: cliconfig.ProviderInstallationDirect,
			tofurcRetries: func() (int, bool) {
				return 0, false
			},
			envVars: map[string]string{
				"TF_PROVIDER_DOWNLOAD_RETRY": "1",
			},
			expectedErrMsg: "/providers/v1/test/exists_0.0.0.zip giving up after 2 attempt(s)",
		},
		"defined tofurc.direct.download_retry_count as 0, no TF_PROVIDER_DOWNLOAD_RETRY, tofurc used": {
			methodType: cliconfig.ProviderInstallationDirect,
			tofurcRetries: func() (int, bool) {
				return 0, true
			},
			envVars:        nil,
			expectedErrMsg: "/providers/v1/test/exists_0.0.0.zip giving up after 1 attempt(s)",
		},
		"defined tofurc.direct.download_retry_count as 1, no TF_PROVIDER_DOWNLOAD_RETRY, tofurc used": {
			methodType: cliconfig.ProviderInstallationDirect,
			tofurcRetries: func() (int, bool) {
				return 1, true
			},
			envVars:        nil,
			expectedErrMsg: "/providers/v1/test/exists_0.0.0.zip giving up after 2 attempt(s)",
		},
		"defined tofurc.direct.download_retry_count as 1, TF_PROVIDER_DOWNLOAD_RETRY defined as 2, tofurc used": {
			methodType: cliconfig.ProviderInstallationDirect,
			tofurcRetries: func() (int, bool) {
				return 1, true
			},
			envVars: map[string]string{
				"TF_PROVIDER_DOWNLOAD_RETRY": "2",
			},
			expectedErrMsg: "/providers/v1/test/exists_0.0.0.zip giving up after 2 attempt(s)",
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			methodType := tt.methodType
			retries := tt.tofurcRetries // This is what simulates the configuration of .tofurc
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			server := httptest.NewServer(registryHandler)
			defer server.Close()
			disco := disco.New(disco.WithHTTPClient(server.Client()))
			disco.ForceHostServices("local.testing", map[string]any{
				"providers.v1": server.URL + "/providers/v1/",
			})

			providerSrc, diags := providerSourceForCLIConfigLocation(t.Context(), methodType, retries, &cliconfig.RegistryProtocolsConfig{}, disco, nil)
			if diags.HasErrors() {
				t.Fatalf("unexpected error creating the provider source: %s", diags)
			}
			provider := addrs.MustParseProviderSourceString("local.testing/test/exists")
			platform := getproviders.Platform{OS: "foo_os", Arch: "bar_arch"}
			packageMeta, err := providerSrc.PackageMeta(t.Context(), provider, getproviders.UnspecifiedVersion, platform)
			if err != nil {
				t.Fatalf("unexpected error getting the package meta information: %s", err)
			}
			// We nullify the authentication since the meaning of this test is not to
			packageMeta.Authentication = nil

			instDir := t.TempDir()
			_, err = packageMeta.Location.InstallProviderPackage(t.Context(), packageMeta, instDir, nil)
			if err == nil {
				t.Fatalf("expected to get an error from the installation of the provider but got nothing")
			}
			if contains := tt.expectedErrMsg; !strings.Contains(err.Error(), contains) {
				t.Fatalf("expected the error from the installation to contain %q but it didn't: %s", contains, err)
			}
		})
	}
}

func TestConfigureProviderDownloadRetry(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string

		expectedConfig getproviders.LocationConfig
	}{
		{
			name: "when no TF_PROVIDER_DOWNLOAD_RETRY env var, default retry attempts used for provider download",
			expectedConfig: getproviders.LocationConfig{
				ProviderDownloadRetries: 2,
			},
		},
		{
			name: "when TF_PROVIDER_DOWNLOAD_RETRY env var configured, it is used provider download",
			envVars: map[string]string{
				"TF_PROVIDER_DOWNLOAD_RETRY": "7",
			},
			expectedConfig: getproviders.LocationConfig{
				ProviderDownloadRetries: 7,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			// Call the function under test
			got := providerSourceLocationConfig(func() (int, bool) {
				return 0, false
			})

			if diff := cmp.Diff(tt.expectedConfig, got); diff != "" {
				t.Fatalf("expected no diff. got:\n%s", diff)
			}
		})
	}
}
