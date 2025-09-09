// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/cliconfig"
	"github.com/opentofu/opentofu/internal/command/cliconfig/ociauthconfig"
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

			err := os.Chdir(originalWorkingDir)
			if err != nil {
				t.Fatalf("Failed to change to original working directory: %v", err)
			}

			// If we have an override directory, change to it (simulating -chdir behavior)
			if overrideWd != "" {
				err := os.Chdir(overrideWd)
				if err != nil {
					t.Fatalf("Failed to change to override directory: %v", err)
				}
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
